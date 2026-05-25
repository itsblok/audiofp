package dsp

import (
	"math"
	"testing"
)

const floatTol = 1e-9

func TestFFT_SingleTone(t *testing.T) {
	// A pure sine at frequency k has all energy in bin k.
	// For N=8, a sine at k=1 completes exactly 1 cycle in 8 samples.
	n := 8
	samples := make([]float64, n)
	for i := range samples {
		// One full cycle of sin across N samples → energy in bin 1
		samples[i] = math.Sin(2 * math.Pi * float64(i) / float64(n))
	}

	bins, err := FFT(samples)
	if err != nil {
		t.Fatalf("FFT error: %v", err)
	}

	// Bin 1 should have significant energy; all others near zero.
	for k, c := range bins {
		mag := math.Sqrt(real(c)*real(c) + imag(c)*imag(c))
		if k == 1 || k == n-1 { // bin 1 and its mirror (N-1)
			if mag < 1.0 {
				t.Errorf("bin %d: expected large magnitude, got %.4f", k, mag)
			}
		} else {
			if mag > floatTol*float64(n) {
				t.Errorf("bin %d: expected ~0, got %.10f", k, mag)
			}
		}
	}
}

func TestFFT_DC(t *testing.T) {
	// A constant signal (DC) has all energy in bin 0.
	n := 16
	samples := make([]float64, n)
	for i := range samples {
		samples[i] = 1.0
	}

	bins, err := FFT(samples)
	if err != nil {
		t.Fatalf("FFT error: %v", err)
	}

	// Bin 0 magnitude should equal N (sum of all ones).
	dc := math.Abs(real(bins[0]))
	if math.Abs(dc-float64(n)) > 1e-6 {
		t.Errorf("DC bin: expected %.1f, got %.6f", float64(n), dc)
	}

	// All other bins should be zero.
	for k := 1; k < n; k++ {
		mag := math.Sqrt(real(bins[k])*real(bins[k]) + imag(bins[k])*imag(bins[k]))
		if mag > 1e-6 {
			t.Errorf("bin %d: expected ~0 for DC input, got %.8f", k, mag)
		}
	}
}

func TestFFT_NotPowerOfTwo(t *testing.T) {
	_, err := FFT(make([]float64, 100))
	if err != ErrNotPowerOfTwo {
		t.Errorf("expected ErrNotPowerOfTwo, got %v", err)
	}
}

func TestFFT_Empty(t *testing.T) {
	_, err := FFT([]float64{})
	if err != ErrNotPowerOfTwo {
		t.Errorf("expected ErrNotPowerOfTwo for empty input, got %v", err)
	}
}

func TestMagnitudeSpectrum_Length(t *testing.T) {
	n := 512
	bins, _ := FFT(make([]float64, n))
	mag := MagnitudeSpectrum(bins)
	expected := n/2 + 1
	if len(mag) != expected {
		t.Errorf("MagnitudeSpectrum length: expected %d, got %d", expected, len(mag))
	}
}

func TestNextPowerOfTwo(t *testing.T) {
	cases := [][2]int{{1, 1}, {2, 2}, {3, 4}, {5, 8}, {1000, 1024}, {1024, 1024}}
	for _, c := range cases {
		got := NextPowerOfTwo(c[0])
		if got != c[1] {
			t.Errorf("NextPowerOfTwo(%d): expected %d, got %d", c[0], c[1], got)
		}
	}
}
