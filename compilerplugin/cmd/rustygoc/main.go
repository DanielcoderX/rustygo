package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"

	"rustygo/compilerplugin"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	// If the first argument is an absolute path to a Go tool (like compile or link),
	// this is being invoked via -toolexec.
	if compilerplugin.IsToolExec(os.Args[1]) {
		if err := compilerplugin.ToolExec(os.Args[1:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
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
	arenaBytes := fs.Int("arena-bytes", 1024*1024, "arena size inserted into rewritten functions")
	output := fs.String("o", "", "build output path")
	explain := fs.Bool("rustygo-explain", false, "print interactive SSA analysis decisions to terminal output")
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

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	args = append([]string{"build", "-toolexec=" + exePath}, buildArgs...)
	args = append(args, patterns...)

	cmd := exec.Command("go", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	env := append(os.Environ(), fmt.Sprintf("RUSTYGO_ARENA_BYTES=%d", *arenaBytes))
	if *explain {
		env = append(env, "RUSTYGO_EXPLAIN=1")
	}
	cmd.Env = env

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go build failed: %w", err)
	}
	return nil
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage:")
	fmt.Fprintln(os.Stderr, "  rustygoc build [flags] ./...")
}
