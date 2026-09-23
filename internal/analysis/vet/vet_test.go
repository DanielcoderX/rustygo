package vet

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestVetAnalyzer(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, Analyzer, "basic", "pragma")
}
