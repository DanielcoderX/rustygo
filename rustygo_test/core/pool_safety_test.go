package rustygo_test

import (
	rg "rustygo"
	"testing"
)

type poolTestObj struct {
	A uint32
	B uint32
}

func TestPoolZeroOnFree(t *testing.T) {
	p := rg.NewPool(
		func() *poolTestObj { return &poolTestObj{} },
		rg.WithZeroOnFree[poolTestObj](),
	)
	obj := p.Alloc()
	obj.A = 10
	obj.B = 20
	p.Free(obj)

	reused := p.Alloc()
	if reused.A != 0 || reused.B != 0 {
		t.Fatalf("expected zeroed object, got A=%d B=%d", reused.A, reused.B)
	}
}

func TestPoolPoisonOnFree(t *testing.T) {
	const poison = byte(0xA5)
	p := rg.NewPool(
		func() *poolTestObj { return &poolTestObj{} },
		rg.WithPoisonOnFree[poolTestObj](poison),
	)

	obj := p.Alloc()
	obj.A = 1
	obj.B = 2
	p.Free(obj)

	reused := p.Alloc()
	if reused.A != 0xA5A5A5A5 || reused.B != 0xA5A5A5A5 {
		t.Fatalf("expected poisoned object, got A=0x%08X B=0x%08X", reused.A, reused.B)
	}
}
