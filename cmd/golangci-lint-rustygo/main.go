package main

import (
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/singlechecker"

	"rustygo/internal/analysis/vet"
)

// New provides the standard plugin constructor required by golangci-lint private linters.
// Usage in .golangci.yml:
//
// linters-settings:
//   custom:
//     rustygovet:
//       path: path/to/golangci-lint-rustygo.so
//       description: Verifies allocation lifetime safety and arena eligibility
//       original-url: https://github.com/DanielcoderX/rustygo
func New(conf any) ([]*analysis.Analyzer, error) {
	return []*analysis.Analyzer{vet.Analyzer}, nil
}

func main() {
	singlechecker.Main(vet.Analyzer)
}
