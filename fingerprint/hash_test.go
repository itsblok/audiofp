package fingerprint

import (
	"math/rand"
	"testing"
)

func TestPackUnpackHash_Roundtrip(t *testing.T) {
	cases := []struct{ a, t_, d int }{
		{0, 0, 0},
		{41, 92, 50},
		{1023, 1023, 4095}, // max values
		{500, 300, 2000},
	}
	for _, c := range cases {
		h := PackHash(c.a, c.t_, c.d)
		ga, gt, gd := UnpackHash(h)
		if ga != c.a || gt != c.t_ || gd != c.d {
			t.Errorf("PackHash(%d,%d,%d) → 0x%08X → UnpackHash(%d,%d,%d)",
				c.a, c.t_, c.d, h, ga, gt, gd)
		}
	}
}

func TestPackHash_Clamping(t *testing.T) {
	// Values exceeding bit width should be clamped, not wrap around.
	h := PackHash(9999, 9999, 99999)
	a, t_, d := UnpackHash(h)
	if a != maxFreqBin {
		t.Errorf("anchor clamping: expected %d, got %d", maxFreqBin, a)
	}
	if t_ != maxFreqBin {
		t.Errorf("target clamping: expected %d, got %d", maxFreqBin, t_)
	}
	if d != maxDeltaFrames {
		t.Errorf("delta clamping: expected %d, got %d", maxDeltaFrames, d)
	}
}

func TestPackHash_Distinctness(t *testing.T) {
	// Different (anchor, target, delta) triples must produce different hashes.
	// Test 1000 random pairs for collisions.
	seen := make(map[uint32]struct{ a, t_, d int })
	rng := rand.New(rand.NewSource(42))
	for i := 0; i < 1000; i++ {
		a := rng.Intn(1024)
		t_ := rng.Intn(1024)
		d := rng.Intn(4096)
		h := PackHash(a, t_, d)
		if prev, exists := seen[h]; exists {
			// A collision is only a bug if the inputs differ.
			if prev.a != a || prev.t_ != t_ || prev.d != d {
				t.Errorf("hash collision: (%d,%d,%d) and (%d,%d,%d) → 0x%08X",
					prev.a, prev.t_, prev.d, a, t_, d, h)
			}
		}
		seen[h] = struct{ a, t_, d int }{a, t_, d}
	}
}

func TestPackHash_NegativeInputs(t *testing.T) {
	// Negative inputs should be clamped to 0, not produce garbage.
	h := PackHash(-1, -5, -100)
	a, t_, d := UnpackHash(h)
	if a != 0 || t_ != 0 || d != 0 {
		t.Errorf("negative input clamping failed: got (%d,%d,%d)", a, t_, d)
	}
}
