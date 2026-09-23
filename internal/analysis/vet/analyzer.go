package vet

import (
	"fmt"
	"reflect"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"

	"rustygo/internal/analysis/escape"
	"rustygo/internal/analysis/pipeline"
)

var (
	violationsOnly bool
)

func init() {
	Analyzer.Flags.BoolVar(&violationsOnly, "violations-only", false, "only report pragma violations (for CI/CD enforcement)")
}

var Analyzer = &analysis.Analyzer{
	Name:       "rustygovet",
	Doc:        "reports allocation lifetime safety and arena eligibility",
	Run:        run,
	Requires:   []*analysis.Analyzer{buildssa.Analyzer},
	ResultType: reflect.TypeOf((*Result)(nil)),
}

type Result struct {
	Decisions []escape.AllocationDecision
}

func run(pass *analysis.Pass) (interface{}, error) {
	ssaData := pass.ResultOf[buildssa.Analyzer].(*buildssa.SSA)
	if ssaData == nil || ssaData.Pkg == nil {
		return nil, nil
	}

	res, err := pipeline.Run(ssaData.Pkg.Prog)
	if err != nil {
		return nil, err
	}

	pragmas := make(map[string]bool)
	for _, f := range pass.Files {
		for _, cg := range f.Comments {
			for _, c := range cg.List {
				text := strings.TrimSpace(c.Text)
				if strings.HasPrefix(text, "//rustygo:arena") || strings.HasPrefix(text, "//rustygo:scoped") {
					pos := pass.Fset.Position(c.Pos())
					key := fmt.Sprintf("%s:%d", pos.Filename, pos.Line)
					pragmas[key] = true
					nextLineKey := fmt.Sprintf("%s:%d", pos.Filename, pos.Line+1)
					pragmas[nextLineKey] = true
				}
			}
		}
	}

	for _, dec := range res.Decisions {
		allocPos := pass.Fset.Position(dec.Allocation.Instruction.Pos())
		key := fmt.Sprintf("%s:%d", allocPos.Filename, allocPos.Line)
		hasPragma := pragmas[key]

		switch dec.Decision {
		case escape.Arena:
			if hasPragma {
				if !violationsOnly {
					pass.Reportf(dec.Allocation.Instruction.Pos(), "[PRAGMA VERIFIED] Allocation of '%s' adheres to //rustygo:arena scope", dec.Allocation.Type.String())
				}
			} else if !violationsOnly {
				pass.Reportf(dec.Allocation.Instruction.Pos(), "[SAFE] Allocation of '%s' is arena-eligible", dec.Allocation.Type.String())
			}
		case escape.Heap:
			if hasPragma {
				pass.Reportf(dec.Allocation.Instruction.Pos(), "[PRAGMA VIOLATION] Allocation of '%s' annotated with //rustygo:arena escapes -> Reason: %v", dec.Allocation.Type.String(), reasonString(dec.Reason))
			} else if !violationsOnly {
				pass.Reportf(dec.Allocation.Instruction.Pos(), "[UNSAFE] Allocation of '%s' escapes -> Reason: %v", dec.Allocation.Type.String(), reasonString(dec.Reason))
			}
		case escape.Unknown:
			if hasPragma {
				pass.Reportf(dec.Allocation.Instruction.Pos(), "[PRAGMA VIOLATION] Allocation of '%s' annotated with //rustygo:arena has unproven lifetime", dec.Allocation.Type.String())
			} else if !violationsOnly {
				pass.Reportf(dec.Allocation.Instruction.Pos(), "[UNKNOWN] Allocation of '%s' lifetime unproven", dec.Allocation.Type.String())
			}
		}
	}

	return &Result{
		Decisions: res.Decisions,
	}, nil
}

func reasonString(r escape.EscapeReason) string {
	switch r {
	case escape.EscapesReturn:
		return "Returned from function scope"
	case escape.EscapesClosure:
		return "Captured by closure"
	case escape.EscapesGoroutine:
		return "Captured by goroutine"
	case escape.EscapesChannel:
		return "Sent through channel"
	case escape.EscapesGlobal:
		return "Stored in global variable"
	case escape.EscapesInterface:
		return "Converted to interface value"
	case escape.EscapesMap:
		return "Stored in map"
	case escape.EscapesSlice:
		return "Escaped via slice"
	case escape.EscapesExternalCall:
		return "Passed to external function call"
	case escape.EscapesUnsafe:
		return "Unsafe or reflection operation"
	default:
		return "Unknown lifetime"
	}
}
