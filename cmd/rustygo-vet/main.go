package main

import (
	"golang.org/x/tools/go/analysis/singlechecker"

	"rustygo/internal/analysis/vet"
)

func main() {
	singlechecker.Main(vet.Analyzer)
}
