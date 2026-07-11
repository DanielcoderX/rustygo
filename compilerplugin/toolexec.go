package compilerplugin

import (
	"fmt"
	"go/ast"
	"go/types"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"rustygo/analyzer"

	"golang.org/x/tools/go/packages"
)

// IsToolExec returns true if the first argument looks like a tool path invoked via -toolexec.
func IsToolExec(arg string) bool {
	base := filepath.Base(arg)
	// Typical tools invoked by go build -toolexec are compile, link, asm, etc.
	// Sometimes they have a .exe extension on Windows.
	base = strings.TrimSuffix(base, ".exe")
	return base == "compile" || base == "link" || base == "asm" || base == "cgo" || base == "vet"
}

func ToolExec(args []string) error {
	toolPath := args[0]
	toolArgs := args[1:]

	if strings.HasSuffix(toolPath, "compile.exe") || strings.HasSuffix(toolPath, "compile") {
		f, _ := os.OpenFile(filepath.Join(os.TempDir(), "rustygoc_env.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if f != nil {
			fmt.Fprintf(f, "--- NEW COMPILE ---\n")
			for _, e := range os.Environ() {
				if strings.Contains(e, "GO") || strings.Contains(e, "LD") || strings.Contains(e, "TOOL") {
					fmt.Fprintf(f, "%s\n", e)
				}
			}
			f.Close()
		}
	}

	toolBase := filepath.Base(toolPath)
	toolBase = strings.TrimSuffix(toolBase, ".exe")

	if toolBase != "compile" {
		// Passthrough for non-compile tools
		return executeTool(toolPath, toolArgs)
	}

	// For compile, we need to extract the .go files, rewrite them, and swap arguments.
	var goFiles []string
	var goFileIndices []int

	for i, arg := range toolArgs {
		if strings.HasSuffix(arg, ".go") {
			goFiles = append(goFiles, arg)
			goFileIndices = append(goFileIndices, i)
		}
	}

	if len(goFiles) == 0 {
		return executeTool(toolPath, toolArgs)
	}

	// Create a temporary directory for rewritten files
	tempDir, err := os.MkdirTemp("", "rustygoc-toolexec-*")
	if err != nil {
		return err
	}
	// We don't defer os.RemoveAll(tempDir) because the compile command might be run later or fail
	// Usually toolexec wrappers can clean up, but let's keep it simple or defer and wait.
	// defer os.RemoveAll(tempDir)

	arenaBytes := 256 * 1024
	if env := os.Getenv("RUSTYGO_ARENA_BYTES"); env != "" {
		if v, err := strconv.Atoi(env); err == nil && v > 0 {
			arenaBytes = v
		}
	}
	rewriteCfg := analyzer.RewriteConfig{ArenaBytes: arenaBytes}

	// We need type information to do safe rewrites.
	// Since we are running on isolated files from the command line, we can load them as a package.
	// This can be tricky if they rely on other files not in this compile invocation,
	// but standard go build passes all package files to compile.
	// Extract the package path being compiled
	var pkgPath string
	for i, arg := range toolArgs {
		if arg == "-p" && i+1 < len(toolArgs) {
			pkgPath = toolArgs[i+1]
			break
		}
	}

	// We ONLY want to rewrite packages in our module, or main.
	// We definitely do not want to rewrite standard library (internal/*, sync/*, runtime/*, etc)
	if pkgPath != "" && !strings.Contains(pkgPath, ".") && !strings.Contains(pkgPath, "rustygo") && pkgPath != "main" && pkgPath != "command-line-arguments" {
		return executeTool(toolPath, toolArgs)
	}

	cfg := &packages.Config{
		Mode: packages.LoadSyntax,
	}
	pkgs, err := packages.Load(cfg, goFiles...)
	if err != nil || len(pkgs) == 0 {
		return fmt.Errorf("packages.Load failed: %v", err)
	}
	
	pkg := pkgs[0]
	
	// Ignore missing function body errors that occur because assembly files aren't in goFiles.
	// But log other errors if we want. For now, we proceed to rewrite.



	funcDecls := make(map[string]*ast.FuncDecl)
	for _, file := range pkg.Syntax {
		ast.Inspect(file, func(n ast.Node) bool {
			if fd, ok := n.(*ast.FuncDecl); ok {
				if obj, ok := pkg.TypesInfo.ObjectOf(fd.Name).(*types.Func); ok {
					funcDecls[obj.FullName()] = fd
				}
				return false
			}
			return true
		})
	}

	newArgs := make([]string, len(toolArgs))
	copy(newArgs, toolArgs)

	type result struct {
		idx     int
		newPath string
		changed bool
		err     error
	}

	resChan := make(chan result, len(pkg.Syntax))
	var wg sync.WaitGroup

	for i, fileAST := range pkg.Syntax {
		wg.Add(1)
		go func(idx int, fAST *ast.File) {
			defer wg.Done()
			origPath := pkg.GoFiles[idx]
			outBytes, changed, err := analyzer.RewriteFileWithConfig(pkg.Fset, fAST, pkg.TypesInfo, pkg.PkgPath, rewriteCfg, funcDecls)
			if err != nil {
				resChan <- result{err: err}
				return
			}
			if changed {
				baseName := filepath.Base(origPath)
				newPath := filepath.Join(tempDir, baseName)
				if err := os.WriteFile(newPath, outBytes, 0644); err != nil {
					resChan <- result{err: err}
					return
				}
				resChan <- result{idx: idx, newPath: newPath, changed: true}
			} else {
				resChan <- result{changed: false}
			}
		}(i, fileAST)
	}

	wg.Wait()
	close(resChan)

	for res := range resChan {
		if res.err != nil {
			return res.err
		}
		if res.changed {
			origPath := pkg.GoFiles[res.idx]
			for _, gIdx := range goFileIndices {
				if toolArgs[gIdx] == origPath {
					newArgs[gIdx] = res.newPath
					break
				}
			}
		}
	}

	return executeTool(toolPath, newArgs)
}

func executeTool(toolPath string, args []string) error {
	cmd := exec.Command(toolPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("tool exec failed: %v", err)
	}
	return nil
}
