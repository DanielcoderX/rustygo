package pragma

func safePragma() {
	//rustygo:arena
	x := new(int) // want `\[PRAGMA VERIFIED\] Allocation of '\*int' adheres to //rustygo:arena scope`
	_ = *x
}

func unsafePragma() *int {
	//rustygo:arena
	x := new(int) // want `\[PRAGMA VIOLATION\] Allocation of '\*int' annotated with //rustygo:arena escapes -> Reason: Returned from function scope`
	return x
}
