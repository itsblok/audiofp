// Package audio — windowing functions for frame preprocessing.
package audio

import "math"

// HannWindow returns a Hann window of length n.
//
// Why Hann? When we slice a continuous signal into frames, we create
// sharp edges at the frame boundaries. These artificial discontinuities
// produce "spectral leakage" — energy from one frequency bin bleeds into
// adjacent bins, smearing the spectrum and making peak picking unreliable.
//
// The Hann window tapers the frame to zero at both ends, eliminating the
// discontinuity. The trade-off is a small loss of frequency resolution
// (wider main lobe), but for fingerprinting purposes this is far preferable
// to leakage artifacts.
//
// Formula: w[n] = 0.5 * (1 - cos(2π*n / (N-1)))
func HannWindow(n int) []float64 {
	w := make([]float64, n)
	for i := range w {
		w[i] = 0.5 * (1.0 - math.Cos(2.0*math.Pi*float64(i)/float64(n-1)))
	}
	return w
}

// ApplyWindow multiplies a frame of samples element-wise by a window function.
// The frame is modified in place. len(frame) must equal len(window).
func ApplyWindow(frame, window []float64) {
	for i := range frame {
		frame[i] *= window[i]
	}
}
