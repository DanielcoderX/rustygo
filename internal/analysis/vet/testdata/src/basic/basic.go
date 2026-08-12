package basic

func f() {
	x := new(int) // want `\[SAFE\] Allocation of '\*int' is arena-eligible`
	_ = *x
}

func g() *int {
	x := new(int) // want `\[UNSAFE\] Allocation of '\*int' escapes -> Reason: Returned from function scope`
	return x
}
