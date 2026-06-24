package dsp

import (
	"math"
	"testing"

	"github.com/cheemney/audiofp/audio"
)

// buildSineSpectrogram is a test helper that generates a spectrogram
// from a pure sine wave at the given frequency.
func buildSineSpectrogram(t *testing.T, freq float64, sampleRate, fftSize int, durationSecs float64) *Spectrogram {
	t.Helper()
	nSamples := int(float64(sampleRate) * durationSecs)
	samples := make([]float64, nSamples)
	for i := range samples {
		samples[i] = math.Sin(2 * math.Pi * freq * float64(i) / float64(sampleRate))
	}
	pcm := audio.PCM{Samples: samples, SampleRate: sampleRate}
	cfg := SpectrogramConfig{FFTSize: fftSize, HopSize: fftSize / 2}
	spec, err := Compute(pcm, cfg)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	return spec
}

func TestExtractPeaks_NonEmpty(t *testing.T) {
	spec := buildSineSpectrogram(t, 1000.0, 44100, 4096, 3.0)
	cfg := DefaultPeakConfig(4096, 44100)
	peaks := ExtractPeaks(spec, cfg)
	if len(peaks) == 0 {
		t.Error("expected at least one peak from a 1kHz sine wave, got none")
	}
}

func TestExtractPeaks_PeakNear1kHz(t *testing.T) {
	const targetFreq = 1000.0
	const sampleRate = 44100
	const fftSize = 4096

	spec := buildSineSpectrogram(t, targetFreq, sampleRate, fftSize, 3.0)
	cfg := DefaultPeakConfig(fftSize, sampleRate)
	peaks := ExtractPeaks(spec, cfg)

	expectedBin := int(math.Round(targetFreq * fftSize / sampleRate))
	tolerance := 3 // bins

	found := false
	for _, p := range peaks {
		if abs(p.FreqBin-expectedBin) <= tolerance {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("no peak within %d bins of 1kHz (expected bin ~%d)", tolerance, expectedBin)
	}
}

func TestExtractPeaks_QuotaRespected(t *testing.T) {
	spec := buildSineSpectrogram(t, 440.0, 44100, 4096, 3.0)
	cfg := DefaultPeakConfig(4096, 44100)
	peaks := ExtractPeaks(spec, cfg)

	// Count peaks per (frame, band) bucket — must not exceed quota.
	type key struct{ frame, band int }
	counts := make(map[key]int)
	bs := cfg.Bands
	for _, p := range peaks {
		b := bs.BandOf(p.FreqBin)
		counts[key{p.TimeFrame, b}]++
	}
	for k, c := range counts {
		if c > cfg.PeaksPerBandPerFrame {
			t.Errorf("frame %d band %d: %d peaks > quota %d",
				k.frame, k.band, c, cfg.PeaksPerBandPerFrame)
		}
	}
}

func TestExtractPeaks_SortedOrder(t *testing.T) {
	spec := buildSineSpectrogram(t, 440.0, 44100, 4096, 3.0)
	cfg := DefaultPeakConfig(4096, 44100)
	peaks := ExtractPeaks(spec, cfg)

	for i := 1; i < len(peaks); i++ {
		a, b := peaks[i-1], peaks[i]
		if a.TimeFrame > b.TimeFrame {
			t.Errorf("peaks not sorted: frame[%d]=%d > frame[%d]=%d", i-1, a.TimeFrame, i, b.TimeFrame)
		}
		if a.TimeFrame == b.TimeFrame && a.FreqBin > b.FreqBin {
			t.Errorf("peaks not sorted within frame: bin[%d]=%d > bin[%d]=%d", i-1, a.FreqBin, i, b.FreqBin)
		}
	}
}

func TestExtractPeaks_EmptySpectrogram(t *testing.T) {
	spec := &Spectrogram{}
	cfg := DefaultPeakConfig(4096, 44100)
	peaks := ExtractPeaks(spec, cfg)
	if len(peaks) != 0 {
		t.Errorf("expected 0 peaks from empty spectrogram, got %d", len(peaks))
	}
}

func TestMedianFloat64(t *testing.T) {
	cases := []struct {
		input    []float64
		expected float64
	}{
		{[]float64{3, 1, 2}, 2.0},
		{[]float64{1, 2, 3, 4}, 2.5},
		{[]float64{5}, 5.0},
	}
	for _, c := range cases {
		got := medianFloat64(c.input)
		if math.Abs(got-c.expected) > 1e-9 {
			t.Errorf("medianFloat64(%v): expected %.2f, got %.2f", c.input, c.expected, got)
		}
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
