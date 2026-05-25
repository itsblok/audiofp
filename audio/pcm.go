// Package audio handles raw PCM ingestion and preprocessing.
// It sits at the boundary between raw audio bytes and the DSP pipeline.
package audio

import (
	"errors"
	"math"
)

// PCM holds a mono, normalized audio signal ready for DSP processing.
// We always convert to mono and float64 internally so the DSP layer
// never needs to worry about channel layouts or integer overflow.
type PCM struct {
	Samples    []float64 // normalized to [-1.0, 1.0]
	SampleRate int       // samples per second (e.g. 44100)
}

// ErrEmptyAudio is returned when no audio samples are provided.
var ErrEmptyAudio = errors.New("audio: empty sample buffer")

// FromInt16Stereo converts raw interleaved int16 stereo PCM to a
// mono float64 PCM. Interleaved means the buffer is [L0, R0, L1, R1, ...].
//
// Downmixing to mono by averaging L+R is standard practice for
// fingerprinting — we don't care about stereo image, only spectral content.
func FromInt16Stereo(raw []int16, sampleRate int) (PCM, error) {
	if len(raw) == 0 {
		return PCM{}, ErrEmptyAudio
	}

	// Each pair of int16 values is one stereo frame.
	numFrames := len(raw) / 2
	samples := make([]float64, numFrames)

	const maxInt16 = float64(math.MaxInt16) // 32767.0

	for i := 0; i < numFrames; i++ {
		left := float64(raw[i*2])
		right := float64(raw[i*2+1])
		// Average channels, then normalize to [-1, 1].
		samples[i] = ((left + right) / 2.0) / maxInt16
	}

	return PCM{Samples: samples, SampleRate: sampleRate}, nil
}

// FromInt16Mono converts raw mono int16 PCM to float64.
// Used when the source is already mono (e.g. phone microphone capture).
func FromInt16Mono(raw []int16, sampleRate int) (PCM, error) {
	if len(raw) == 0 {
		return PCM{}, ErrEmptyAudio
	}

	const maxInt16 = float64(math.MaxInt16)
	samples := make([]float64, len(raw))

	for i, s := range raw {
		samples[i] = float64(s) / maxInt16
	}

	return PCM{Samples: samples, SampleRate: sampleRate}, nil
}

// Duration returns the length of the audio in seconds.
func (p PCM) Duration() float64 {
	if p.SampleRate == 0 {
		return 0
	}
	return float64(len(p.Samples)) / float64(p.SampleRate)
}

// NumSamples returns the total number of audio samples.
func (p PCM) NumSamples() int {
	return len(p.Samples)
}
