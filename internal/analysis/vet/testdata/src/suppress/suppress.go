package suppress

// test that //rustygo:trust suppresses escape diagnostics
func trustEscape() *int {
	//rustygo:trust
	x := new(int) // want `\[TRUSTED\] Allocation of '\*int' exempted via pragma`
	return x
}

func ignoreEscape() *int {
	//rustygo:ignore
	x := new(int) // want `\[TRUSTED\] Allocation of '\*int' exempted via pragma`
	return x
}
