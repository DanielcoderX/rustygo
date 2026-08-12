package summary

import (
	"golang.org/x/tools/go/ssa"
)

// ParamSummary describes the escape and flow behavior of a function parameter.
type ParamSummary struct {
	Index    int
	Name     string
	Escapes  bool
	Returned bool
	Stored   bool
}

// ReturnSummary describes the escape and flow behavior of a function return value.
type ReturnSummary struct {
	Index            int
	DerivedFromParam int // -1 if not directly derived from a parameter
	Escapes          bool
}

// FunctionSummary aggregates inter-procedural flow summaries for a function.
type FunctionSummary struct {
	Function   *ssa.Function
	FullName   string
	Params     []ParamSummary
	Returns    []ReturnSummary
	HasBody    bool
	IsExternal bool
}

// SummaryMap indexes function summaries by their full package-qualified name.
type SummaryMap map[string]*FunctionSummary
