package rustygo_test

import (
	"testing"

	rg "rustygo"
)

func TestAutoTunerCalibration(t *testing.T) {
	tuner := rg.NewAutoTuner(4096, 1024*1024)

	key := "test_handler_process"
	if size := tuner.RecommendedSize(key); size != 4096 {
		t.Fatalf("expected initial 4096, got %d", size)
	}

	// Calibrate with 64KB usage
	tuner.Calibrate(key, 64*1024)
	recommended := tuner.RecommendedSize(key)
	if recommended < 64*1024 {
		t.Fatalf("expected recommendation >= 64KB, got %d", recommended)
	}

	// Repeated calibration settles into headroom
	for i := 0; i < 5; i++ {
		tuner.Calibrate(key, 128*1024)
	}
	newRec := tuner.RecommendedSize(key)
	if newRec < 128*1024 {
		t.Fatalf("expected recommendation >= 128KB, got %d", newRec)
	}
}

func TestWithAutoTunedScope(t *testing.T) {
	key := "benchmark_user_pipeline"

	for i := 0; i < 10; i++ {
		err := rg.WithAutoTunedScope(key, func(s *rg.Scope) error {
			slice := rg.AllocSlice[byte](s, 32*1024)
			slice[0] = 42
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	rec := rg.GlobalAutoTuner.RecommendedSize(key)
	if rec < 32*1024 {
		t.Fatalf("expected learned capacity >= 32KB, got %d", rec)
	}
}
