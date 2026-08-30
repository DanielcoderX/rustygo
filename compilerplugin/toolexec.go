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
	"rustygo/internal/analysis/escape"
	"rustygo/internal/analysis/pipeline"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
)

// IsToolExec returns true if the first argument looks like a tool path invoked via -toolexec.
func IsToolExec(arg string) bool {
	base := filepath.Base(arg)
	// Typical tools invoked by go build -toolexec are compile, link, asm, etc.
	// Sometimes they have a .exe extension on Windows.
	base = strings.TrimSuffix(base, ".exe")
	return base == "compile" || base == "link" || base == "asm" || base == "cgo" || base == "vet"
}

func ToolExec(args []string) (err error) {
	toolPath := args[0]
	toolArgs := args[1:]

	toolBase := filepath.Base(toolPath)
	toolBase = strings.TrimSuffix(toolBase, ".exe")

	if toolBase != "compile" {
		return executeTool(toolPath, toolArgs)
	}

	debug := os.Getenv("RUSTYGO_DEBUG") == "1"

	defer func() {
		if r := recover(); r != nil {
			if debug {
				fmt.Fprintf(os.Stderr, "[rustygo-debug] panic recovered: %v\n", r)
			}
			err = executeTool(toolPath, toolArgs)
		}
	}()

	err = runRewriteAndCompile(toolPath, toolArgs, debug)
	if err != nil {
		if debug {
			fmt.Fprintf(os.Stderr, "[rustygo-debug] rewrite failed, falling back: %v\n", err)
		}
		return executeTool(toolPath, toolArgs)
	}
	return nil
}

func runRewriteAndCompile(toolPath string, toolArgs []string, debug bool) error {
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

	tempDir, err := os.MkdirTemp("", "rustygoc-toolexec-*")
	if err != nil {
		return err
	}

	arenaBytes := 256 * 1024
	if env := os.Getenv("RUSTYGO_ARENA_BYTES"); env != "" {
		if v, err := strconv.Atoi(env); err == nil && v > 0 {
			arenaBytes = v
		}
	}
	rewriteCfg := analyzer.RewriteConfig{ArenaBytes: arenaBytes}

	var pkgPath string
	for i, arg := range toolArgs {
		if arg == "-p" && i+1 < len(toolArgs) {
			pkgPath = toolArgs[i+1]
			break
		}
	}

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
	
	if os.Getenv("RUSTYGO_EXPLAIN") != "" {
		prog := ssa.NewProgram(pkg.Fset, 0)
		ssaPkg := prog.CreatePackage(pkg.Types, pkg.Syntax, pkg.TypesInfo, true)
		ssaPkg.Build()
		res, err := pipeline.Run(prog)
		if err == nil {
			for _, dec := range res.Decisions {
				pos := pkg.Fset.Position(dec.Allocation.Instruction.Pos())
				switch dec.Decision {
				case escape.Arena:
					fmt.Printf("[SAFE]   %s:%d: Allocation of '%s' -> Bound to Thread-Local Arena\n", filepath.Base(pos.Filename), pos.Line, dec.Allocation.Type.String())
				case escape.Heap:
					fmt.Printf("[UNSAFE] %s:%d: Allocation of '%s' Escapes -> Reason: %v\n", filepath.Base(pos.Filename), pos.Line, dec.Allocation.Type.String(), dec.Reason)
				case escape.Unknown:
					fmt.Printf("[UNKNOWN] %s:%d: Allocation of '%s' -> Lifetime unproven\n", filepath.Base(pos.Filename), pos.Line, dec.Allocation.Type.String())
				}
			}
		}
	}

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
			outBytes, changed, err := analyzer.RewriteFileWithConfig(pkg.Fset, fAST, pkg.Syntax, pkg.TypesInfo, pkg.PkgPath, rewriteCfg, funcDecls)
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
