package rustygo_test

import (
	rg "rustygo"
	"testing"
	"unsafe"
)

func FuzzArenaAllocAlignedOrErr(f *testing.F) {
	f.Add(128, 16, 8)
	f.Add(1024, 64, 64)
	f.Add(64, 33, 4)
	f.Add(256, 0, 16)
	f.Add(256, 16, 3)

	f.Fuzz(func(t *testing.T, arenaSize int, n int, align int) {
		if arenaSize <= 0 {
			arenaSize = 1
		}
		if arenaSize > 1<<20 {
			arenaSize = 1 << 20
		}
		if n > 1<<20 {
			n = 1 << 20
		}
		if n < -1<<20 {
			n = -1 << 20
		}
		if align > 1<<16 {
			align = 1 << 16
		}
		if align < -1<<16 {
			align = -1 << 16
		}

		arena := rg.NewArena(arenaSize)
		buf, err := arena.AllocAlignedOrErr(n, align)
		if err != nil {
			return
		}
		if len(buf) != n {
			t.Fatalf("len mismatch: got=%d want=%d", len(buf), n)
		}
		if align > 0 && (align&(align-1)) == 0 {
			addr := uintptr(unsafe.Pointer(&buf[0]))
			if addr%uintptr(align) != 0 {
				t.Fatalf("alignment mismatch: align=%d addr=%d", align, addr)
			}
		}
	})
}

func FuzzArenaMarkRewind(f *testing.F) {
	f.Add(128, 16, 8, 12)
	f.Add(64, 32, 8, 48)
	f.Add(256, 64, 128, 0)

	f.Fuzz(func(t *testing.T, arenaSize int, n1 int, n2 int, rewindTo int) {
		if arenaSize <= 0 {
			arenaSize = 1
		}
		if arenaSize > 1<<20 {
			arenaSize = 1 << 20
		}
		clamp := func(v int) int {
			if v < 0 {
				return 0
			}
			if v > arenaSize {
				return arenaSize
			}
			return v
		}

		n1 = clamp(n1)
		n2 = clamp(n2)
		rewindTo = clamp(rewindTo)

		arena := rg.NewArena(arenaSize)
		if n1 > 0 {
			_, _ = arena.TryAlloc(n1)
		}
		mark := arena.Mark()
		if n2 > 0 {
			_, _ = arena.TryAlloc(n2)
		}

		_ = arena.Rewind(rg.ArenaMark(rewindTo))
		_ = arena.Rewind(mark)
		used, capc := arena.Stats()
		if used < 0 || used > capc {
			t.Fatalf("invalid stats after rewind: used=%d cap=%d", used, capc)
		}
	})
}
