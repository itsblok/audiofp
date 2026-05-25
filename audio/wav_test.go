package audio

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

// synthSine creates a PCM containing a pure sine wave at the given frequency.
func synthSine(freq float64, sampleRate int, durationSecs float64) PCM {
	n := int(float64(sampleRate) * durationSecs)
	samples := make([]float64, n)
	for i := range samples {
		samples[i] = math.Sin(2 * math.Pi * freq * float64(i) / float64(sampleRate))
	}
	return PCM{Samples: samples, SampleRate: sampleRate}
}

func TestWAV_RoundTrip(t *testing.T) {
	// Write a 440 Hz sine to WAV, read it back, verify the samples match.
	original := synthSine(440.0, 44100, 1.0)

	dir := t.TempDir()
	path := filepath.Join(dir, "test.wav")

	if err := WriteWAV(path, original); err != nil {
		t.Fatalf("WriteWAV: %v", err)
	}

	decoded, err := ReadWAV(path)
	if err != nil {
		t.Fatalf("ReadWAV: %v", err)
	}

	if decoded.SampleRate != original.SampleRate {
		t.Errorf("sample rate: want %d, got %d", original.SampleRate, decoded.SampleRate)
	}
	if len(decoded.Samples) != len(original.Samples) {
		t.Fatalf("sample count: want %d, got %d", len(original.Samples), len(decoded.Samples))
	}

	// 16-bit quantization introduces at most 1/32767 ≈ 0.00003 error per sample.
	const maxQuantizationError = 1.0 / float64(math.MaxInt16) * 1.5
	maxErr := 0.0
	for i, got := range decoded.Samples {
		err := math.Abs(got - original.Samples[i])
		if err > maxErr {
			maxErr = err
		}
	}
	if maxErr > maxQuantizationError {
		t.Errorf("max sample error %.6f exceeds 16-bit quantization tolerance %.6f",
			maxErr, maxQuantizationError)
	}
}

func TestWAV_SampleRatePreserved(t *testing.T) {
	for _, sr := range []int{8000, 22050, 44100, 48000} {
		pcm := synthSine(440.0, sr, 0.5)
		dir := t.TempDir()
		path := filepath.Join(dir, "test.wav")

		WriteWAV(path, pcm)
		decoded, err := ReadWAV(path)
		if err != nil {
			t.Errorf("sr=%d: ReadWAV: %v", sr, err)
			continue
		}
		if decoded.SampleRate != sr {
			t.Errorf("sr=%d: decoded sample rate = %d", sr, decoded.SampleRate)
		}
	}
}

func TestWAV_NotAWAVFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "garbage.wav")
	os.WriteFile(path, []byte("this is not a WAV file at all"), 0644)

	_, err := ReadWAV(path)
	if err != ErrNotWAV {
		t.Errorf("expected ErrNotWAV, got %v", err)
	}
}

func TestWAV_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.wav")
	os.WriteFile(path, []byte{}, 0644)

	_, err := ReadWAV(path)
	if err == nil {
		t.Error("expected error for empty file")
	}
}

func TestWAV_SilencePreserved(t *testing.T) {
	// A silent signal (all zeros) should round-trip to all zeros.
	pcm := PCM{Samples: make([]float64, 4096), SampleRate: 44100}

	dir := t.TempDir()
	path := filepath.Join(dir, "silence.wav")
	WriteWAV(path, pcm)

	decoded, err := ReadWAV(path)
	if err != nil {
		t.Fatalf("ReadWAV: %v", err)
	}
	for i, s := range decoded.Samples {
		if s != 0.0 {
			t.Errorf("sample[%d]: expected 0, got %f", i, s)
		}
	}
}

func TestWAV_FileSize(t *testing.T) {
	// Verify the WAV file has the expected byte count.
	// 16-bit mono: fileSize = 44 (header) + nSamples*2.
	pcm := synthSine(440.0, 44100, 1.0) // 44100 samples
	dir := t.TempDir()
	path := filepath.Join(dir, "test.wav")
	WriteWAV(path, pcm)

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	expectedSize := int64(44 + len(pcm.Samples)*2)
	if info.Size() != expectedSize {
		t.Errorf("file size: expected %d bytes, got %d", expectedSize, info.Size())
	}
}

func TestWAV_Clipping(t *testing.T) {
	// Samples outside [-1, 1] must be clipped to prevent integer wrap-around.
	pcm := PCM{
		Samples:    []float64{2.0, -3.0, 1.5, -1.5},
		SampleRate: 44100,
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "clipped.wav")
	WriteWAV(path, pcm)

	decoded, err := ReadWAV(path)
	if err != nil {
		t.Fatalf("ReadWAV: %v", err)
	}
	for i, s := range decoded.Samples {
		if s < -1.0 || s > 1.0 {
			t.Errorf("sample[%d] = %f: expected clipped to [-1,1]", i, s)
		}
	}
}
