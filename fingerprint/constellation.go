// Package fingerprint builds and operates on audio fingerprints.
// It consumes DSP-layer peaks and transforms them into matchable structures.
//
// The constellation map is the bridge between raw signal processing and
// the hashing/matching logic. It is intentionally kept as a lightweight
// value type — no database concerns, no I/O, pure data.
package fingerprint

import (
	"sort"

	"github.com/itsblok/audiofp/dsp"
)

// Point is a single landmark in the constellation map.
// It represents a (time, frequency) coordinate where the spectrogram
// had a locally prominent peak.
//
// We discard MagnitudeDB deliberately: the hash must be magnitude-invariant
// so that a quiet recording matches a loud one of the same song.
type Point struct {
	TimeFrame int // frame index in the spectrogram
	FreqBin   int // FFT bin index
}

// Constellation is the sparse time-frequency landmark representation
// of an audio signal.
//
// Robustness properties:
//   - Invariant to overall volume (magnitude discarded)
//   - Invariant to DC offset (phase-free)
//   - Stable under mild additive noise (local maxima persist)
//   - Stable under mild EQ changes (dominant peaks shift little)
type Constellation []Point

// FromPeaks converts DSP-layer peaks into a Constellation.
// This is the boundary crossing from the dsp package to the fingerprint package.
func FromPeaks(peaks []dsp.Peak) Constellation {
	c := make(Constellation, len(peaks))
	for i, p := range peaks {
		c[i] = Point{
			TimeFrame: p.TimeFrame,
			FreqBin:   p.FreqBin,
		}
	}
	return c
}

// Len returns the number of landmarks in the constellation.
func (c Constellation) Len() int {
	return len(c)
}

// TimeSpan returns the first and last TimeFrame indices in the constellation.
func (c Constellation) TimeSpan() (first, last int) {
	if len(c) == 0 {
		return 0, 0
	}
	first, last = c[0].TimeFrame, c[0].TimeFrame
	for _, p := range c[1:] {
		if p.TimeFrame < first {
			first = p.TimeFrame
		}
		if p.TimeFrame > last {
			last = p.TimeFrame
		}
	}
	return first, last
}

// PointsInTimeRange returns all points whose TimeFrame is in [lo, hi] inclusive.
func (c Constellation) PointsInTimeRange(lo, hi int) Constellation {
	var out Constellation
	for _, p := range c {
		if p.TimeFrame >= lo && p.TimeFrame <= hi {
			out = append(out, p)
		}
	}
	return out
}

// SortedByTime returns a new constellation sorted by TimeFrame ascending.
func (c Constellation) SortedByTime() Constellation {
	out := make(Constellation, len(c))
	copy(out, c)
	sort.Slice(out, func(i, j int) bool {
		if out[i].TimeFrame != out[j].TimeFrame {
			return out[i].TimeFrame < out[j].TimeFrame
		}
		return out[i].FreqBin < out[j].FreqBin
	})
	return out
}

// BandDistribution counts how many points fall in each frequency band.
// bandOf maps FreqBin → band index (returns -1 for out-of-range bins).
// A healthy constellation should have coverage across all bands.
func (c Constellation) BandDistribution(bandOf func(bin int) int) map[int]int {
	dist := make(map[int]int)
	for _, p := range c {
		b := bandOf(p.FreqBin)
		if b >= 0 {
			dist[b]++
		}
	}
	return dist
}
