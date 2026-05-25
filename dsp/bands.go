// Package dsp — logarithmic frequency band partitioning.
//
// Why log bands?
// Human pitch perception is logarithmic — the octave from 100→200 Hz sounds
// like the same "distance" as 1000→2000 Hz. Linear FFT bins allocate equal
// numbers of bins per Hz, which means low frequencies get very few bins while
// high frequencies get thousands.
//
// For fingerprinting we want peak candidates spread evenly across what humans
// perceive as the audible spectrum. Log bands enforce this: each band covers
// one "perceptual slice" of the spectrum, and we pick a fixed number of peaks
// from each band regardless of how many linear bins it contains.
package dsp

import (
	"math"
)

// FrequencyBand represents a contiguous range of FFT bins.
// BinLo and BinHi are inclusive indices into the magnitude spectrum.
type FrequencyBand struct {
	Index  int
	BinLo  int
	BinHi  int
	FreqLo float64 // Hz
	FreqHi float64 // Hz
}

// NumBins returns the number of FFT bins in this band.
func (b FrequencyBand) NumBins() int {
	return b.BinHi - b.BinLo + 1
}

// BandSet is a precomputed collection of bands with a fast bin→band lookup.
type BandSet struct {
	Bands     []FrequencyBand
	binToBand []int // index = bin, value = band index (-1 if out of range)
	totalBins int
}

// LogBands partitions [minFreq, maxFreq] into numBands logarithmically spaced
// bands, then maps each partition to FFT bin indices.
//
// Parameters:
//   numBands   — number of frequency bands (typically 6)
//   minFreq    — lower bound in Hz (typically 300 Hz, below which music has
//                little fingerprint-useful content and more noise)
//   maxFreq    — upper bound in Hz (typically 10000 Hz — most musical energy)
//   fftSize    — FFT frame size (e.g. 4096)
//   sampleRate — audio sample rate (e.g. 44100)
func LogBands(numBands int, minFreq, maxFreq float64, fftSize, sampleRate int) BandSet {
	totalBins := fftSize/2 + 1
	binToBand := make([]int, totalBins)
	for i := range binToBand {
		binToBand[i] = -1 // outside all bands by default
	}

	// Hz → bin index: bin = freq * fftSize / sampleRate
	freqToBin := func(freq float64) int {
		bin := int(math.Round(freq * float64(fftSize) / float64(sampleRate)))
		if bin < 0 {
			return 0
		}
		if bin >= totalBins {
			return totalBins - 1
		}
		return bin
	}

	bands := make([]FrequencyBand, numBands)

	// Compute log-spaced frequency boundaries.
	logMin := math.Log(minFreq)
	logMax := math.Log(maxFreq)
	logStep := (logMax - logMin) / float64(numBands)

	for i := 0; i < numBands; i++ {
		fLo := math.Exp(logMin + float64(i)*logStep)
		fHi := math.Exp(logMin + float64(i+1)*logStep)

		bLo := freqToBin(fLo)
		bHi := freqToBin(fHi)

		// Ensure at least one bin per band (can happen for very narrow bands).
		if bHi < bLo {
			bHi = bLo
		}

		bands[i] = FrequencyBand{
			Index:  i,
			BinLo:  bLo,
			BinHi:  bHi,
			FreqLo: fLo,
			FreqHi: fHi,
		}

		// Populate reverse lookup: bin → band index.
		for b := bLo; b <= bHi && b < totalBins; b++ {
			binToBand[b] = i
		}
	}

	return BandSet{
		Bands:     bands,
		binToBand: binToBand,
		totalBins: totalBins,
	}
}

// DefaultBands returns the standard 6-band log partition for music fingerprinting.
// Frequency range 300–10000 Hz covers most of the musically rich spectrum
// while avoiding sub-bass rumble and ultrasonic noise.
func DefaultBands(fftSize, sampleRate int) BandSet {
	return LogBands(6, 200.0, 10000.0, fftSize, sampleRate)
}

// BandOf returns the band index for a given FFT bin.
// Returns -1 if the bin falls outside all defined bands.
func (bs BandSet) BandOf(bin int) int {
	if bin < 0 || bin >= len(bs.binToBand) {
		return -1
	}
	return bs.binToBand[bin]
}
