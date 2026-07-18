package indicators_test

import (
	"testing"

	"github.com/Nalin-Angrish/Navier-Stocks/src/analysts/quantitative/indicators"
)

// ---------------------------------------------------------------------------
// VolumeBreakout
// ---------------------------------------------------------------------------

func TestVolumeBreakout_Breakout(t *testing.T) {
	// 5 prior values average to 100, current = 200 → ratio = 2.0
	volumes := []int64{100, 110, 90, 105, 95, 200}
	r := indicators.VolumeBreakout(volumes, 5, 2.0)
	if !r.IsBreakout {
		t.Fatal("expected breakout")
	}
	if r.CurrentVolume != 200 {
		t.Fatalf("CurrentVolume = %d, want 200", r.CurrentVolume)
	}
	if r.VolumeSMA != 100 {
		t.Fatalf("VolumeSMA = %f, want 100", r.VolumeSMA)
	}
	if r.Ratio != 2.0 {
		t.Fatalf("Ratio = %f, want 2.0", r.Ratio)
	}
	if r.Window != 5 {
		t.Fatalf("Window = %d, want 5", r.Window)
	}
}

func TestVolumeBreakout_NoBreakout(t *testing.T) {
	volumes := []int64{100, 110, 90, 105, 95, 150}
	r := indicators.VolumeBreakout(volumes, 5, 2.0)
	if r.IsBreakout {
		t.Fatal("expected no breakout")
	}
	if r.CurrentVolume != 150 {
		t.Fatalf("CurrentVolume = %d, want 150", r.CurrentVolume)
	}
	if r.VolumeSMA != 100 {
		t.Fatalf("VolumeSMA = %f, want 100", r.VolumeSMA)
	}
	if r.Ratio != 1.5 {
		t.Fatalf("Ratio = %f, want 1.5", r.Ratio)
	}
}

func TestVolumeBreakout_ExactThreshold(t *testing.T) {
	// ratio exactly equals threshold → breakout
	volumes := []int64{100, 100, 100, 100, 100, 200}
	r := indicators.VolumeBreakout(volumes, 5, 2.0)
	if !r.IsBreakout {
		t.Fatal("expected breakout at exact threshold")
	}
}

func TestVolumeBreakout_BarelyBelowThreshold(t *testing.T) {
	volumes := []int64{100, 100, 100, 100, 100, 199}
	r := indicators.VolumeBreakout(volumes, 5, 2.0)
	if r.IsBreakout {
		t.Fatal("expected no breakout (199/100 < 2.0)")
	}
}

func TestVolumeBreakout_WindowLargerThanHistory(t *testing.T) {
	// 2 prior values, window=10, should use all 2 prior
	volumes := []int64{50, 50, 150}
	r := indicators.VolumeBreakout(volumes, 10, 2.0)
	if !r.IsBreakout {
		t.Fatal("expected breakout using available history")
	}
	if r.VolumeSMA != 50 {
		t.Fatalf("VolumeSMA = %f, want 50", r.VolumeSMA)
	}
	if r.Ratio != 3.0 {
		t.Fatalf("Ratio = %f, want 3.0", r.Ratio)
	}
}

func TestVolumeBreakout_InsufficientHistory(t *testing.T) {
	// only 1 volume → cannot compute
	volumes := []int64{100}
	r := indicators.VolumeBreakout(volumes, 5, 2.0)
	if r.IsBreakout {
		t.Fatal("expected no breakout with insufficient history")
	}
}

func TestVolumeBreakout_SingleElement(t *testing.T) {
	r := indicators.VolumeBreakout([]int64{100}, 5, 2.0)
	if r.IsBreakout {
		t.Fatal("expected no breakout with single element")
	}
}

func TestVolumeBreakout_Empty(t *testing.T) {
	r := indicators.VolumeBreakout(nil, 5, 2.0)
	if r.IsBreakout {
		t.Fatal("expected no breakout with empty input")
	}
}

func TestVolumeBreakout_ZeroWindow(t *testing.T) {
	volumes := []int64{100, 200, 300}
	r := indicators.VolumeBreakout(volumes, 0, 2.0)
	if r.IsBreakout {
		t.Fatal("expected no breakout with zero window")
	}
}

func TestVolumeBreakout_NegativeWindow(t *testing.T) {
	volumes := []int64{100, 200, 300}
	r := indicators.VolumeBreakout(volumes, -1, 2.0)
	if r.IsBreakout {
		t.Fatal("expected no breakout with negative window")
	}
}

func TestVolumeBreakout_ZeroThreshold(t *testing.T) {
	volumes := []int64{100, 200, 300}
	r := indicators.VolumeBreakout(volumes, 2, 0)
	if r.IsBreakout {
		t.Fatal("expected no breakout with zero threshold")
	}
}

func TestVolumeBreakout_NegativeThreshold(t *testing.T) {
	volumes := []int64{100, 200, 300}
	r := indicators.VolumeBreakout(volumes, 2, -1.0)
	if r.IsBreakout {
		t.Fatal("expected no breakout with negative threshold")
	}
}

func TestVolumeBreakout_AllZeroVolumes(t *testing.T) {
	volumes := []int64{0, 0, 0, 0, 0, 0}
	r := indicators.VolumeBreakout(volumes, 5, 2.0)
	if r.IsBreakout {
		t.Fatal("expected no breakout with all zero volumes")
	}
	if r.CurrentVolume != 0 {
		t.Fatalf("CurrentVolume = %d, want 0", r.CurrentVolume)
	}
}

func TestVolumeBreakout_SmallWindow(t *testing.T) {
	// window=1 → SMA of single prior value = 100
	// current = 300, ratio = 3.0
	volumes := []int64{100, 300}
	r := indicators.VolumeBreakout(volumes, 1, 2.0)
	if !r.IsBreakout {
		t.Fatal("expected breakout with window=1")
	}
	if r.VolumeSMA != 100 {
		t.Fatalf("VolumeSMA = %f, want 100", r.VolumeSMA)
	}
}

func TestVolumeBreakout_CurrentEqualToPrior(t *testing.T) {
	volumes := []int64{100, 100, 100}
	r := indicators.VolumeBreakout(volumes, 2, 2.0)
	if r.IsBreakout {
		t.Fatal("expected no breakout (same as average)")
	}
	if r.Ratio != 1.0 {
		t.Fatalf("Ratio = %f, want 1.0", r.Ratio)
	}
}

func TestVolumeBreakout_CustomThreshold(t *testing.T) {
	volumes := []int64{10, 10, 10, 10, 10, 15}
	r := indicators.VolumeBreakout(volumes, 5, 1.2)
	if !r.IsBreakout {
		t.Fatal("expected breakout with threshold=1.2")
	}
}

// ---------------------------------------------------------------------------
// Benchmarks
// ---------------------------------------------------------------------------

func BenchmarkVolumeBreakout(b *testing.B) {
	volumes := make([]int64, 1000)
	for i := range volumes {
		volumes[i] = int64(1000 + i)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		indicators.VolumeBreakout(volumes, 10, 2.0)
	}
}
