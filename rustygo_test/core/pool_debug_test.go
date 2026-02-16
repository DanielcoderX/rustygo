//go:build rustygo_debug

package rustygo_test

import (
	rg "rustygo"
	"strings"
	"testing"
)

func TestPoolDebugDetectsDoubleFree(t *testing.T) {
	p := rg.NewPool(func() *int { v := 0; return &v })
	obj := p.Alloc()
	p.Free(obj)

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic on double free")
		}
		msg, ok := r.(string)
		if !ok || !strings.Contains(msg, "double free") {
			t.Fatalf("unexpected panic: %v", r)
		}
	}()
	p.Free(obj)
}
