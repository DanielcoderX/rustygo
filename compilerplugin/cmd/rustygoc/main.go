package main

import (
	"flag"
	"fmt"
	"os"

	"rustygo/compilerplugin"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "build":
		if err := runBuild(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	default:
		usage()
		os.Exit(1)
	}
}

func runBuild(args []string) error {
	fs := flag.NewFlagSet("rustygoc build", flag.ContinueOnError)
	arenaBytes := fs.Int("arena-bytes", 64*1024, "arena size inserted into rewritten functions")
	keepWork := fs.Bool("work", false, "keep and print the temporary rewritten module directory")
	output := fs.String("o", "", "build output path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	patterns := fs.Args()
	if len(patterns) == 0 {
		return fmt.Errorf("usage: rustygoc build [flags] ./...")
	}

	var buildArgs []string
	if *output != "" {
		buildArgs = append(buildArgs, "-o", *output)
	}

	result, err := compilerplugin.Build(compilerplugin.Config{
		ArenaBytes: *arenaBytes,
		Stdout:     os.Stdout,
		Stderr:     os.Stderr,
	}, patterns, buildArgs)
	if err != nil {
		if result != nil && *keepWork {
			fmt.Fprintf(os.Stderr, "work dir: %s\n", result.TempModuleRoot)
		}
		return err
	}

	fmt.Fprintf(os.Stdout, "rewrote %d file(s)\n", len(result.RewrittenFiles))
	if *keepWork {
		fmt.Fprintf(os.Stdout, "work dir: %s\n", result.TempModuleRoot)
	}
	return nil
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage:")
	fmt.Fprintln(os.Stderr, "  rustygoc build [flags] ./...")
}
