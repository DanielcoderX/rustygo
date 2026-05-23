// Command rustygocheck runs the rustygo arena eligibility analyzer.
package main

import (
	"flag"
	"fmt"
	"os"

	"rustygo/analyzer"

	"golang.org/x/tools/go/analysis/singlechecker"
	"golang.org/x/tools/go/packages"
)

func main() {
	if hasFixFlag(os.Args[1:]) {
		if err := runFixMode(os.Args[1:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	singlechecker.Main(analyzer.Analyzer)
}

func runFixMode(args []string) error {
	fs := flag.NewFlagSet("rustygocheck", flag.ContinueOnError)
	fix := fs.Bool("fix", false, "rewrite eligible new/make sites to rustygo arena calls")
	arenaBytes := fs.Int("arena-bytes", 64*1024, "arena size inserted into rewritten functions")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if !*fix {
		return nil
	}
	patterns := fs.Args()
	if len(patterns) == 0 {
		return fmt.Errorf("usage: rustygocheck -fix ./...")
	}

	cfg := &packages.Config{
		Mode: packages.LoadAllSyntax,
	}
	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		return err
	}
	if packages.PrintErrors(pkgs) > 0 {
		return fmt.Errorf("package load failed")
	}

	changed := 0
	rewriteCfg := analyzer.RewriteConfig{ArenaBytes: *arenaBytes}
	for _, pkg := range pkgs {
		for i, file := range pkg.Syntax {
			out, ok, err := analyzer.RewriteFileWithConfig(pkg.Fset, file, pkg.TypesInfo, pkg.PkgPath, rewriteCfg)
			if err != nil {
				return err
			}
			if !ok {
				continue
			}
			if err := os.WriteFile(pkg.GoFiles[i], out, 0o644); err != nil {
				return err
			}
			changed++
			fmt.Printf("rewrote %s\n", pkg.GoFiles[i])
		}
	}

	if changed == 0 {
		fmt.Println("no eligible rewrites found")
	}
	return nil
}

func hasFixFlag(args []string) bool {
	for _, arg := range args {
		if arg == "-fix" || arg == "--fix" || arg == "-fix=true" {
			return true
		}
	}
	return false
}
