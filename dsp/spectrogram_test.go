package dsp

import (
	"math"
	"testing"

	"github.com/cheemney/audiofp/audio"
)

func TestCompute_FrameCount(t *testing.T) {
	// With FFTSize=4096, HopSize=2048, and exactly 2*4096 samples,
	// we expect floor((2*4096 - 4096) / 2048) + 1 = 3 frames.
	cfg := SpectrogramConfig{FFTSize: 4096, HopSize: 2048}
	nSamples := 2 * cfg.FFTSize
	samples := make([]float64, nSamples)

	pcm := audio.PCM{Samples: samples, SampleRate: 44100}
	spec, err := Compute(pcm, cfg)
	if err != nil {
		t.Fatalf("Compute error: %v", err)
	}

	// Frames start at 0, 2048, 4096 (last valid start before 8192-4096=4096)
	expectedFrames := (nSamples-cfg.FFTSize)/cfg.HopSize + 1
	if len(spec.Frames) != expectedFrames {
		t.Errorf("frame count: expected %d, got %d", expectedFrames, len(spec.Frames))
	}
}

func TestCompute_BinCount(t *testing.T) {
	cfg := SpectrogramConfig{FFTSize: 512, HopSize: 256}
	samples := make([]float64, 1024)
	pcm := audio.PCM{Samples: samples, SampleRate: 44100}
	spec, _ := Compute(pcm, cfg)

	expectedBins := cfg.FFTSize/2 + 1
	for i, frame := range spec.Frames {
		if len(frame.Magnitude) != expectedBins {
			t.Errorf("frame %d: expected %d bins, got %d", i, expectedBins, len(frame.Magnitude))
		}
	}
}

func TestCompute_ToneLocalization(t *testing.T) {
	// Inject a 1kHz sine and verify the spectrogram peak is near 1kHz.
	sampleRate := 44100
	cfg := SpectrogramConfig{FFTSize: 4096, HopSize: 2048}
	targetFreq := 1000.0

	nSamples := cfg.FFTSize * 4
	samples := make([]float64, nSamples)
	for i := range samples {
		samples[i] = math.Sin(2 * math.Pi * targetFreq * float64(i) / float64(sampleRate))
	}

	pcm := audio.PCM{Samples: samples, SampleRate: sampleRate}
	spec, err := Compute(pcm, cfg)
	if err != nil {
		t.Fatalf("Compute error: %v", err)
	}

	// Check the middle frame (less susceptible to edge effects).
	midFrame := spec.Frames[len(spec.Frames)/2]
	peakBin := 0
	peakMag := 0.0
	for bin, mag := range midFrame.Magnitude {
		if mag > peakMag {
			peakMag = mag
			peakBin = bin
		}
	}

	peakFreq := spec.FrequencyOfBin(peakBin)
	freqError := math.Abs(peakFreq - targetFreq)
	// Allow error up to 2 bin widths (2 * sampleRate/FFTSize ≈ 21.5 Hz)
	maxError := 2.0 * float64(sampleRate) / float64(cfg.FFTSize)
	if freqError > maxError {
		t.Errorf("tone localization error %.2f Hz > tolerance %.2f Hz (peak at %.2f Hz)",
			freqError, maxError, peakFreq)
	}
}

func TestTimeOfFrame(t *testing.T) {
	spec := &Spectrogram{
		Config:     SpectrogramConfig{FFTSize: 4096, HopSize: 2048},
		SampleRate: 44100,
	}
	// Frame 1 starts at sample 2048
	expected := float64(2048) / 44100.0
	got := spec.TimeOfFrame(1)
	if math.Abs(got-expected) > 1e-9 {
		t.Errorf("TimeOfFrame(1): expected %.6f, got %.6f", expected, got)
	}
}
