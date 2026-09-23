package rustygo

import (
	"runtime"
	"strconv"
	"sync"
)

const (
	defaultMinSlab = 4 * 1024       // 4 KB
	defaultMaxSlab = 64 * 1024 * 1024 // 64 MB
	headroomFactor = 1.25            // 25% extra headroom
)

type siteStats struct {
	estimatedBytes int
	samples        int
}

// AutoTuner monitors allocation metrics across call sites to dynamically calibrate optimal arena slab sizes.
type AutoTuner struct {
	mu      sync.RWMutex
	sites   map[string]*siteStats
	minSlab int
	maxSlab int
}

// GlobalAutoTuner is the process-wide default arena auto-tuner.
var GlobalAutoTuner = NewAutoTuner(defaultMinSlab, defaultMaxSlab)

// NewAutoTuner creates a new auto-tuner with boundary limits.
func NewAutoTuner(minSlab, maxSlab int) *AutoTuner {
	if minSlab <= 0 {
		minSlab = defaultMinSlab
	}
	if maxSlab < minSlab {
		maxSlab = defaultMaxSlab
	}
	return &AutoTuner{
		sites:   make(map[string]*siteStats),
		minSlab: minSlab,
		maxSlab: maxSlab,
	}
}

// Calibrate updates the learned memory capacity for a given site key.
func (t *AutoTuner) Calibrate(key string, usedBytes int) {
	if usedBytes <= 0 {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	stat, exists := t.sites[key]
	if !exists {
		t.sites[key] = &siteStats{
			estimatedBytes: usedBytes,
			samples:        1,
		}
		return
	}

	stat.samples++
	// Exponential moving average: 0.7 old + 0.3 new
	newEst := int(float64(stat.estimatedBytes)*0.7 + float64(usedBytes)*0.3)
	stat.estimatedBytes = newEst
}

// RecommendedSize returns the calibrated arena slab size for key.
func (t *AutoTuner) RecommendedSize(key string) int {
	t.mu.RLock()
	stat, exists := t.sites[key]
	t.mu.RUnlock()

	if !exists {
		return t.minSlab
	}

	target := int(float64(stat.estimatedBytes) * headroomFactor)
	if target < t.minSlab {
		target = t.minSlab
	}
	if target > t.maxSlab {
		target = t.maxSlab
	}

	// Align to 4KB page boundary
	target = (target + 4095) & ^4095
	return target
}

// CallerSiteKey generates a unique call site key from the caller's PC.
func CallerSiteKey(skip int) string {
	pc, _, _, ok := runtime.Caller(skip + 1)
	if !ok {
		return "unknown_site"
	}
	return "pc:" + strconv.FormatUint(uint64(pc), 16)
}

// NewAutoTunedArena creates an Arena sized according to learned peak memory usage for key.
func NewAutoTunedArena(key string) *Arena {
	size := GlobalAutoTuner.RecommendedSize(key)
	return NewArena(size)
}

// WithAutoTunedScope runs fn with an automatically sized arena calibrated to caller location.
func WithAutoTunedScope(key string, fn func(s *Scope) error) error {
	if key == "" {
		key = CallerSiteKey(1)
	}

	size := GlobalAutoTuner.RecommendedSize(key)
	arena := NewArena(size)
	defer arena.Close()

	scope := arena.EnterScope()
	defer scope.Exit()

	err := fn(scope)
	GlobalAutoTuner.Calibrate(key, scope.UsedBytes())
	return err
}
