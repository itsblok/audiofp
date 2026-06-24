// Package dsp — Short-Time Fourier Transform and spectrogram construction.
package dsp

import (
	"github.com/cheemney/audiofp/audio"
)

// SpectrogramConfig controls how the STFT is computed.
// These parameters directly affect the time-frequency resolution trade-off.
type SpectrogramConfig struct {
	// FFTSize is the number of samples per frame (must be power of two).
	// Larger = better frequency resolution, worse time resolution.
	// 4096 @ 44100Hz ≈ 93ms per frame, frequency resolution ≈ 10.8 Hz/bin.
	FFTSize int

	// HopSize is the step between successive frames (samples).
	// HopSize < FFTSize creates overlapping frames.
	// 50% overlap (HopSize = FFTSize/2) is the standard for fingerprinting.
	HopSize int
}

// DefaultConfig returns the standard fingerprinting configuration.
// These values are tuned for music at 44100 Hz sample rate.
func DefaultConfig() SpectrogramConfig {
	return SpectrogramConfig{
		FFTSize: 4096,
		HopSize: 2048,
	}
}

// Frame represents one time slice of the spectrogram.
// TimeIndex is the frame number (multiply by HopSize/SampleRate for seconds).
type Frame struct {
	TimeIndex int
	Magnitude []float64 // magnitude per frequency bin, length = FFTSize/2+1
}

// Spectrogram holds the full time-frequency representation of an audio signal.
type Spectrogram struct {
	Frames     []Frame
	Config     SpectrogramConfig
	SampleRate int
}

// NumBins returns the number of frequency bins per frame.
func (s *Spectrogram) NumBins() int {
	return s.Config.FFTSize/2 + 1
}

// TimeOfFrame converts a frame index to time in seconds.
func (s *Spectrogram) TimeOfFrame(frameIdx int) float64 {
	if s.SampleRate == 0 {
		return 0
	}
	return float64(frameIdx*s.Config.HopSize) / float64(s.SampleRate)
}

// FrequencyOfBin converts a bin index to frequency in Hz.
func (s *Spectrogram) FrequencyOfBin(bin int) float64 {
	if s.Config.FFTSize == 0 || s.SampleRate == 0 {
		return 0
	}
	return float64(bin) * float64(s.SampleRate) / float64(s.Config.FFTSize)
}

// Compute builds a Spectrogram from a PCM signal using the given config.
//
// The STFT works by:
//  1. Slicing the signal into overlapping frames of length FFTSize
//  2. Applying a Hann window to each frame (suppress boundary artifacts)
//  3. Computing the FFT of each windowed frame
//  4. Keeping only the magnitude of the positive-frequency bins
//
// The result is a 2D array where rows = time frames, columns = frequency bins.
// Each cell contains the energy of that frequency at that moment in time.
func Compute(pcm audio.PCM, cfg SpectrogramConfig) (*Spectrogram, error) {
	window := audio.HannWindow(cfg.FFTSize)
	samples := pcm.Samples
	nSamples := len(samples)

	var frames []Frame
	frameIdx := 0

	for start := 0; start+cfg.FFTSize <= nSamples; start += cfg.HopSize {
		// Extract frame and apply window — modifies a copy, not the original.
		frame := make([]float64, cfg.FFTSize)
		copy(frame, samples[start:start+cfg.FFTSize])
		audio.ApplyWindow(frame, window)

		// Compute FFT.
		bins, err := FFT(frame)
		if err != nil {
			return nil, err
		}

		// Convert complex bins to magnitude spectrum.
		mag := MagnitudeSpectrum(bins)

		frames = append(frames, Frame{
			TimeIndex: frameIdx,
			Magnitude: mag,
		})
		frameIdx++
	}

	return &Spectrogram{
		Frames:     frames,
		Config:     cfg,
		SampleRate: pcm.SampleRate,
	}, nil
}
