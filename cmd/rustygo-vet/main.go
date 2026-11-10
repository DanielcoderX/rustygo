package main

import (
	"golang.org/x/tools/go/analysis/singlechecker"

	"github.com/DanielcoderX/rustygo/internal/analysis/vet"
)

func main() {
	singlechecker.Main(vet.Analyzer)
}
