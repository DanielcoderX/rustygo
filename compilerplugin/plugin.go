package compilerplugin

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"rustygo/analyzer"

	"golang.org/x/tools/go/packages"
)

type Config struct {
	WorkDir    string
	ArenaBytes int
	Stdout     io.Writer
	Stderr     io.Writer
}

type Result struct {
	ModuleRoot     string
	TempModuleRoot string
	RewrittenFiles []string
}

func RewriteModule(cfg Config, patterns []string) (*Result, error) {
	if len(patterns) == 0 {
		return nil, fmt.Errorf("no package patterns provided")
	}

	workDir, err := resolveWorkDir(cfg.WorkDir)
	if err != nil {
		return nil, err
	}
	moduleRoot, err := moduleRoot(workDir)
	if err != nil {
		return nil, err
	}
	tempRoot, err := os.MkdirTemp("", "rustygoc-module-*")
	if err != nil {
		return nil, err
	}
	if err := copyModuleTree(moduleRoot, tempRoot); err != nil {
		return nil, err
	}

	pkgs, err := packages.Load(&packages.Config{
		Mode: packages.LoadAllSyntax,
		Dir:  moduleRoot,
	}, patterns...)
	if err != nil {
		return nil, err
	}
	if packages.PrintErrors(pkgs) > 0 {
		return nil, fmt.Errorf("package load failed")
	}

	result := &Result{
		ModuleRoot:     moduleRoot,
		TempModuleRoot: tempRoot,
	}
	rewriteCfg := analyzer.RewriteConfig{ArenaBytes: cfg.ArenaBytes}

	for _, pkg := range pkgs {
		for i, file := range pkg.Syntax {
			out, changed, err := analyzer.RewriteFileWithConfig(pkg.Fset, file, pkg.TypesInfo, pkg.PkgPath, rewriteCfg)
			if err != nil {
				return nil, err
			}
			if !changed {
				continue
			}

			rel, err := filepath.Rel(moduleRoot, pkg.GoFiles[i])
			if err != nil {
				return nil, err
			}
			dst := filepath.Join(tempRoot, rel)
			if err := os.WriteFile(dst, out, 0o644); err != nil {
				return nil, err
			}
			result.RewrittenFiles = append(result.RewrittenFiles, dst)
		}
	}

	return result, nil
}

func Build(cfg Config, patterns []string, buildArgs []string) (*Result, error) {
	result, err := RewriteModule(cfg, patterns)
	if err != nil {
		return nil, err
	}

	args := append([]string{"build"}, buildArgs...)
	args = append(args, patterns...)
	cmd := exec.Command("go", args...)
	cmd.Dir = result.TempModuleRoot
	cmd.Stdout = cfg.stdoutOrDiscard()
	cmd.Stderr = cfg.stderrOrDiscard()
	if err := cmd.Run(); err != nil {
		return result, err
	}

	return result, nil
}

func resolveWorkDir(workDir string) (string, error) {
	if workDir != "" {
		return filepath.Abs(workDir)
	}
	return os.Getwd()
}

func moduleRoot(workDir string) (string, error) {
	cmd := exec.Command("go", "env", "GOMOD")
	cmd.Dir = workDir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	goMod := strings.TrimSpace(string(out))
	if goMod == "" || goMod == os.DevNull {
		return "", fmt.Errorf("workdir %q is not inside a Go module", workDir)
	}
	return filepath.Dir(goMod), nil
}

func copyModuleTree(srcRoot, dstRoot string) error {
	return filepath.WalkDir(srcRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(srcRoot, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if shouldSkip(rel, d) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		dst := filepath.Join(dstRoot, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		return copyFile(path, dst)
	})
}

func shouldSkip(rel string, d fs.DirEntry) bool {
	name := d.Name()
	if name == ".git" || name == ".idea" || name == ".vscode" {
		return true
	}
	if strings.HasPrefix(name, ".") && d.IsDir() {
		return true
	}
	if strings.HasSuffix(name, ".exe") || strings.HasSuffix(name, ".test") {
		return true
	}
	if strings.Contains(rel, string(filepath.Separator)+".git"+string(filepath.Separator)) {
		return true
	}
	return false
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func (cfg Config) stdoutOrDiscard() io.Writer {
	if cfg.Stdout != nil {
		return cfg.Stdout
	}
	return io.Discard
}

func (cfg Config) stderrOrDiscard() io.Writer {
	if cfg.Stderr != nil {
		return cfg.Stderr
	}
	return io.Discard
}
