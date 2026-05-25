// Package dsp implements core digital signal processing operations.
// It is intentionally decoupled from audio I/O and fingerprinting logic.
package dsp

import (
	"errors"
	"math"
	"math/cmplx"
)

// ErrNotPowerOfTwo is returned when FFT input length is not a power of two.
var ErrNotPowerOfTwo = errors.New("dsp: FFT input length must be a power of two")

// FFT computes the Discrete Fourier Transform of the input using the
// Cooley-Tukey radix-2 Decimation-In-Time algorithm.
//
// Why FFT and not DFT?
// A naive DFT is O(N²). The FFT achieves O(N log N) by recursively
// decomposing the N-point DFT into two N/2-point DFTs (even and odd indices),
// then combining them with "twiddle factors" (complex exponentials).
// For N=4096, this is ~50x faster than naive DFT — crucial for real-time use.
//
// The input is real-valued audio samples. The output is complex-valued:
// each bin k contains the amplitude and phase of frequency k*Fs/N.
//
// Input is copied before transform; the original slice is not modified.
// Input length must be a power of two.
func FFT(input []float64) ([]complex128, error) {
	n := len(input)
	if n == 0 || (n&(n-1)) != 0 {
		return nil, ErrNotPowerOfTwo
	}

	// Promote real input to complex. Imaginary part is zero.
	x := make([]complex128, n)
	for i, v := range input {
		x[i] = complex(v, 0)
	}

	fftInPlace(x)
	return x, nil
}

// fftInPlace performs the Cooley-Tukey radix-2 DIT FFT in place.
// This is the standard butterfly implementation.
func fftInPlace(x []complex128) {
	n := len(x)
	if n <= 1 {
		return
	}

	// Bit-reversal permutation: reorder input so that recursive
	// even/odd decomposition maps to sequential memory access.
	bitReverse(x)

	// Iterative butterfly computation.
	// We build up from size-2 sub-problems to the full N-point DFT.
	for size := 2; size <= n; size <<= 1 {
		halfSize := size / 2
		// Twiddle factor step: w = e^(-2πi/size)
		// This is the unit root that rotates by one "size"-th of a full circle.
		wStep := cmplx.Exp(complex(0, -2.0*math.Pi/float64(size)))

		for k := 0; k < n; k += size {
			w := complex(1, 0) // start at w^0 = 1
			for j := 0; j < halfSize; j++ {
				// Butterfly: combine even sample x[k+j] with
				// twiddle-rotated odd sample x[k+j+halfSize].
				u := x[k+j]
				v := w * x[k+j+halfSize]
				x[k+j] = u + v
				x[k+j+halfSize] = u - v
				w *= wStep // advance twiddle factor
			}
		}
	}
}

// bitReverse permutes x so that element at index i moves to bit-reverse(i).
// This is required by the DIT FFT to correctly interleave even/odd samples.
func bitReverse(x []complex128) {
	n := len(x)
	bits := 0
	for tmp := n; tmp > 1; tmp >>= 1 {
		bits++
	}

	for i := 0; i < n; i++ {
		j := reverseBits(i, bits)
		if i < j {
			x[i], x[j] = x[j], x[i]
		}
	}
}

// reverseBits reverses the lower `bits` bits of x.
func reverseBits(x, bits int) int {
	result := 0
	for i := 0; i < bits; i++ {
		result = (result << 1) | (x & 1)
		x >>= 1
	}
	return result
}

// MagnitudeSpectrum returns the magnitude of each FFT bin.
// We only return the first N/2+1 bins (the positive frequencies).
// The upper half is a mirror image for real-valued input (conjugate symmetry).
//
// Magnitude = sqrt(Re² + Im²) = absolute value of the complex bin.
// This discards phase information, which is intentional: fingerprints
// should be phase-invariant so that time-shifted recordings still match.
func MagnitudeSpectrum(bins []complex128) []float64 {
	// Only the first N/2+1 bins are unique for real input.
	n := len(bins)/2 + 1
	mag := make([]float64, n)
	for i := 0; i < n; i++ {
		mag[i] = cmplx.Abs(bins[i])
	}
	return mag
}

// NextPowerOfTwo returns the smallest power of two >= n.
// Useful for zero-padding frames to FFT-compatible lengths.
func NextPowerOfTwo(n int) int {
	if n <= 0 {
		return 1
	}
	p := 1
	for p < n {
		p <<= 1
	}
	return p
}
