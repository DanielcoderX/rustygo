package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

type BenchResult struct {
	Name      string
	Iterations int64
	NsPerOp   float64
	BytesPerOp int64
	AllocsPerOp int64
}

var benchRegex = regexp.MustCompile(`^(Benchmark\S+)\s+(\d+)\s+([\d\.]+)\s+ns/op(?:\s+(\d+)\s+B/op)?(?:\s+(\d+)\s+allocs/op)?`)

func main() {
	args := os.Args[1:]
	for i, a := range args {
		if a == "--" {
			args = args[i+1:]
			break
		}
	}

	fs := flag.NewFlagSet("rustygo-bench", flag.ExitOnError)
	benchTime := fs.String("benchtime", "1s", "Benchmark iteration duration")
	pkgPath := fs.String("pkg", "./codec", "Package pattern to benchmark")
	outFile := fs.String("out", "", "Output markdown file path (default stdout)")
	_ = fs.Parse(args)

	fmt.Printf("🚀 Running benchmarks for %s (benchtime=%s)...\n", *pkgPath, *benchTime)

	cmd := exec.Command("go", "test", "-bench=.", "-benchmem", "-run=^$", "-benchtime="+*benchTime, *pkgPath)
	cmd.Stderr = os.Stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating pipe: %v\n", err)
		os.Exit(1)
	}

	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting benchmark command: %v\n", err)
		os.Exit(1)
	}

	var results []BenchResult
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		fmt.Println("  " + line)

		matches := benchRegex.FindStringSubmatch(line)
		if len(matches) > 3 {
			iters, _ := strconv.ParseInt(matches[2], 10, 64)
			ns, _ := strconv.ParseFloat(matches[3], 64)
			var bPerOp, allocsPerOp int64
			if len(matches) > 4 && matches[4] != "" {
				bPerOp, _ = strconv.ParseInt(matches[4], 10, 64)
			}
			if len(matches) > 5 && matches[5] != "" {
				allocsPerOp, _ = strconv.ParseInt(matches[5], 10, 64)
			}

			results = append(results, BenchResult{
				Name:        cleanBenchName(matches[1]),
				Iterations:  iters,
				NsPerOp:     ns,
				BytesPerOp:  bPerOp,
				AllocsPerOp: allocsPerOp,
			})
		}
	}

	_ = cmd.Wait()

	if len(results) == 0 {
		fmt.Println("No benchmark results captured.")
		return
	}

	// Generate Markdown Table
	var sb strings.Builder
	sb.WriteString("\n## 📊 Benchmark Results\n\n")
	sb.WriteString("| Benchmark | Iterations | Latency (`ns/op`) | Memory (`B/op`) | Allocations (`allocs/op`) |\n")
	sb.WriteString("| :--- | :--- | :--- | :--- | :--- |\n")

	for _, r := range results {
		sb.WriteString(fmt.Sprintf("| **`%s`** | %d | **%.2f ns** | %d B | %d allocs |\n",
			r.Name, r.Iterations, r.NsPerOp, r.BytesPerOp, r.AllocsPerOp))
	}

	output := sb.String()
	fmt.Print(output)

	if *outFile != "" {
		if err := os.WriteFile(*outFile, []byte(output), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to write %s: %v\n", *outFile, err)
		} else {
			fmt.Printf("Wrote report to %s\n", *outFile)
		}
	}
}

func cleanBenchName(name string) string {
	// Trim Benchmark prefix and CPU suffix like -16
	idx := strings.LastIndex(name, "-")
	if idx > 0 {
		name = name[:idx]
	}
	return strings.TrimPrefix(name, "Benchmark")
}
