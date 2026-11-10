package pipeline

import (
	"golang.org/x/tools/go/ssa"

	"github.com/DanielcoderX/rustygo/internal/analysis/allocation"
	"github.com/DanielcoderX/rustygo/internal/analysis/escape"
	"github.com/DanielcoderX/rustygo/internal/analysis/lifetime"
	"github.com/DanielcoderX/rustygo/internal/analysis/summary"
)

// AnalysisResult holds the compiled findings from the complete analysis pipeline.
type AnalysisResult struct {
	Allocations []allocation.Allocation
	Lifetimes   lifetime.LifetimeResult
	Decisions   []escape.AllocationDecision
}

// Run executes the complete analysis pipeline sequentially over an SSA program.
func Run(program *ssa.Program) (*AnalysisResult, error) {
	// 1. Function Summaries
	summary.AnalyzeSummaries(program)

	// 2. Allocation Discovery
	allocRes, err := allocation.Analyze(program)
	if err != nil {
		return nil, err
	}

	// 3. Lifetime Analysis
	lifetimeRes, err := lifetime.Analyze(program)
	if err != nil {
		return nil, err
	}

	// 3. Escape Classification
	decisions := escape.Classify(allocRes, lifetimeRes)

	var allocList []allocation.Allocation
	for _, a := range allocRes.Allocations {
		allocList = append(allocList, *a)
	}

	return &AnalysisResult{
		Allocations: allocList,
		Lifetimes:   *lifetimeRes,
		Decisions:   decisions,
	}, nil
}
