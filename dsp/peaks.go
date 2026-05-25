// Package dsp — spectral peak extraction from a spectrogram.
//
// Peak picking is the core of Shazam's robustness. The key insight:
// instead of comparing entire spectrograms (fragile under noise, EQ changes,
// recording level shifts), we compare only the *positions* of prominent local
// maxima. These positions are stable under mild distortion because they
// represent the frequencies where the most energy is concentrated, and those
// dominant frequencies don't move much when you add noise or change volume.
package dsp

import (
	"math"
	"sort"
)

// PeakConfig controls the peak extraction algorithm.
type PeakConfig struct {
	// TimeNeighborhood is the ±frame radius for local max detection.
	// A peak at frame t must exceed all frames in [t-N, t+N].
	TimeNeighborhood int

	// FreqNeighborhood is the ±bin radius for local max detection.
	// Prevents adjacent harmonics from each generating peaks.
	FreqNeighborhood int

	// PeaksPerBandPerFrame is the maximum number of peaks to keep
	// from each frequency band in each time frame.
	PeaksPerBandPerFrame int

	// MinDBAboveFloor is the minimum dB a candidate must exceed
	// relative to the median energy of its frame.
	MinDBAboveFloor float64

	// Bands defines the log-frequency partition used for balanced selection.
	Bands BandSet
}

// DefaultPeakConfig returns well-tested defaults for 44100 Hz / FFT 4096 audio.
func DefaultPeakConfig(fftSize, sampleRate int) PeakConfig {
	return PeakConfig{
		TimeNeighborhood:     2,
		FreqNeighborhood:     5,
		PeaksPerBandPerFrame: 5,
		MinDBAboveFloor:      10.0,
		Bands:                DefaultBands(fftSize, sampleRate),
	}
}

// Peak represents a single spectral landmark: a point in time-frequency
// space where energy is locally maximal and above the noise floor.
type Peak struct {
	TimeFrame   int
	FreqBin     int
	MagnitudeDB float64
}

// ExtractPeaks finds spectral peaks from a Spectrogram.
//
// Algorithm:
//  1. Convert each frame's linear magnitude to dB scale.
//  2. Compute per-frame adaptive noise floor (median dB).
//  3. Test each bin as 2D local maximum in time x frequency window.
//  4. Accept only candidates exceeding (noiseFloor + MinDBAboveFloor).
//  5. Per band per frame, keep only the top PeaksPerBandPerFrame by magnitude.
func ExtractPeaks(spec *Spectrogram, cfg PeakConfig) []Peak {
	nFrames := len(spec.Frames)
	if nFrames == 0 {
		return nil
	}

	// Pre-convert all frames to dB for efficient neighborhood lookups.
	dbFrames := make([][]float64, nFrames)
	for i, frame := range spec.Frames {
		dbFrames[i] = toDBFrame(frame.Magnitude)
	}

	// Index candidates by (frameIdx, bandIdx) for quota enforcement.
	type bucketKey struct{ frame, band int }
	buckets := make(map[bucketKey][]Peak)

	for t := 0; t < nFrames; t++ {
		frame := dbFrames[t]
		nBins := len(frame)

		// Adaptive threshold: median dB + MinDBAboveFloor.
		// This makes peak detection signal-relative, not absolute —
		// so it works equally well on loud and quiet recordings.
		noiseFloor := medianFloat64(frame)
		threshold := noiseFloor + cfg.MinDBAboveFloor

		for f := 0; f < nBins; f++ {
			val := frame[f]
			if val < threshold {
				continue // fast reject
			}

			if !isLocalMax2D(dbFrames, t, f, cfg.TimeNeighborhood, cfg.FreqNeighborhood, nFrames, nBins) {
				continue
			}

			bandIdx := cfg.Bands.BandOf(f)
			if bandIdx < 0 {
				continue // outside fingerprint frequency range
			}

			key := bucketKey{t, bandIdx}
			buckets[key] = append(buckets[key], Peak{
				TimeFrame:   t,
				FreqBin:     f,
				MagnitudeDB: val,
			})
		}
	}

	// Enforce per-band-per-frame quota: keep top K by magnitude.
	var peaks []Peak
	for _, candidates := range buckets {
		sort.Slice(candidates, func(i, j int) bool {
			return candidates[i].MagnitudeDB > candidates[j].MagnitudeDB
		})
		limit := cfg.PeaksPerBandPerFrame
		if limit > len(candidates) {
			limit = len(candidates)
		}
		peaks = append(peaks, candidates[:limit]...)
	}

	// Deterministic output order: time ascending, then frequency ascending.
	sort.Slice(peaks, func(i, j int) bool {
		if peaks[i].TimeFrame != peaks[j].TimeFrame {
			return peaks[i].TimeFrame < peaks[j].TimeFrame
		}
		return peaks[i].FreqBin < peaks[j].FreqBin
	})

	return peaks
}

// isLocalMax2D returns true if dbFrames[t][f] is strictly greater than all
// other values in the (2*dt+1) x (2*df+1) neighborhood around (t, f).
// Window edges are clamped to valid index ranges.
func isLocalMax2D(dbFrames [][]float64, t, f, dt, df, nFrames, nBins int) bool {
	val := dbFrames[t][f]

	tLo := t - dt
	if tLo < 0 {
		tLo = 0
	}
	tHi := t + dt
	if tHi >= nFrames {
		tHi = nFrames - 1
	}
	fLo := f - df
	if fLo < 0 {
		fLo = 0
	}
	fHi := f + df
	if fHi >= nBins {
		fHi = nBins - 1
	}

	for ti := tLo; ti <= tHi; ti++ {
		for fi := fLo; fi <= fHi; fi++ {
			if ti == t && fi == f {
				continue
			}
			if dbFrames[ti][fi] >= val {
				return false
			}
		}
	}
	return true
}

// toDBFrame converts linear magnitude to dB: 20*log10(mag + epsilon).
// Compresses 1000:1 amplitude ratios into a ~60 dB range for threshold math.
func toDBFrame(magnitude []float64) []float64 {
	const epsilon = 1e-10
	db := make([]float64, len(magnitude))
	for i, m := range magnitude {
		db[i] = 20.0 * math.Log10(m+epsilon)
	}
	return db
}

// medianFloat64 returns the median of a float64 slice without mutating it.
func medianFloat64(data []float64) float64 {
	if len(data) == 0 {
		return 0
	}
	sorted := make([]float64, len(data))
	copy(sorted, data)
	sort.Float64s(sorted)
	mid := len(sorted) / 2
	if len(sorted)%2 == 0 {
		return (sorted[mid-1] + sorted[mid]) / 2.0
	}
	return sorted[mid]
}
