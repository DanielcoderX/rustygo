package lifetime

import (
	"fmt"
	"sync/atomic"
)

var lastResult atomic.Pointer[LifetimeResult]

// DumpGraph prints the details of the constructed lifetime flow graph.
func DumpGraph() {
	res := lastResult.Load()
	if res == nil {
		fmt.Println("No analysis result available.")
		return
	}
	fmt.Println("=== Lifetime Flow Graph ===")
	for id, report := range res.Reports {
		fmt.Printf("Allocation %s:\n", id)
		for _, ref := range report.References {
			fmt.Printf("  -> %s\n", ref)
		}
	}
}

// DumpRegions prints the lexical region hierarchy.
func DumpRegions() {
	res := lastResult.Load()
	if res == nil {
		fmt.Println("No analysis result available.")
		return
	}
	fmt.Println("=== Lexical Regions ===")
	for id, report := range res.Reports {
		if report.Region != nil {
			fmt.Printf("Allocation %s Region: %s (Start: %d, End: %d)\n", id, report.Region.Name, report.Region.Start, report.Region.End)
		}
	}
}

// DumpOwnership prints the ownership tracking details.
func DumpOwnership() {
	res := lastResult.Load()
	if res == nil {
		fmt.Println("No analysis result available.")
		return
	}
	fmt.Println("=== Ownership Allocation Map ===")
	for id, report := range res.Reports {
		fmt.Printf("Allocation %s Owner Scope: %s\n", id, report.OwnerScope)
	}
}

// DumpDiagnostics prints the final analysis results and violations.
func DumpDiagnostics() {
	res := lastResult.Load()
	if res == nil {
		fmt.Println("No analysis result available.")
		return
	}
	fmt.Println("=== Lifetime Diagnostics ===")
	for id, report := range res.Reports {
		fmt.Printf("Object: %s\n", id)
		fmt.Printf("  State: %s\n", report.LifetimeState)
		if report.LifetimeState != Safe {
			fmt.Printf("  Reason: %s\n", report.EscapeReason)
			fmt.Printf("  Violations: %v\n", report.LifetimeViolations)
		}
		fmt.Println()
	}
}
