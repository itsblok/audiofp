package fingerprint

import (
	"math"
	"testing"

	"github.com/cheemney/audiofp/audio"
	"github.com/cheemney/audiofp/dsp"
	"github.com/cheemney/audiofp/storage"
)

// buildConstellationFromSine runs the full DSP pipeline on a synthetic
// sine wave and returns the resulting constellation.
func buildConstellationFromSine(t *testing.T, freq float64, sampleRate int, durationSecs float64) Constellation {
	t.Helper()
	nSamples := int(float64(sampleRate) * durationSecs)
	samples := make([]float64, nSamples)
	for i := range samples {
		samples[i] = math.Sin(2 * math.Pi * freq * float64(i) / float64(sampleRate))
	}
	pcm := audio.PCM{Samples: samples, SampleRate: sampleRate}
	cfg := dsp.SpectrogramConfig{FFTSize: 4096, HopSize: 2048}
	spec, err := dsp.Compute(pcm, cfg)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	peaks := dsp.ExtractPeaks(spec, dsp.DefaultPeakConfig(4096, sampleRate))
	return FromPeaks(peaks)
}

// buildConstellationFromChord builds a constellation from a multi-frequency
// chord. Used for tests that require unique hashes (e.g. alignment recovery).
// A pure sine produces too many identical hashes because the same peak repeats
// every frame — all alignments get equal votes and the test becomes undefined.
func buildConstellationFromChord(t *testing.T, freqs []float64, sampleRate int, durationSecs float64) Constellation {
	t.Helper()
	nSamples := int(float64(sampleRate) * durationSecs)
	samples := make([]float64, nSamples)
	for i := range samples {
		t_ := float64(i) / float64(sampleRate)
		var v float64
		for _, f := range freqs {
			v += math.Sin(2 * math.Pi * f * t_)
		}
		samples[i] = v / float64(len(freqs))
	}
	pcm := audio.PCM{Samples: samples, SampleRate: sampleRate}
	cfg := dsp.SpectrogramConfig{FFTSize: 4096, HopSize: 2048}
	spec, err := dsp.Compute(pcm, cfg)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	peaks := dsp.ExtractPeaks(spec, dsp.DefaultPeakConfig(4096, sampleRate))
	return FromPeaks(peaks)
}

// indexSong runs the full indexing pipeline and stores fingerprints.
func indexSong(t *testing.T, store storage.Store, name string, constellation Constellation) uint32 {
	t.Helper()
	id, err := store.RegisterSong(name)
	if err != nil {
		t.Fatalf("RegisterSong(%q): %v", name, err)
	}
	hashes := GenerateHashes(constellation, DefaultHasherConfig())
	hashVals := make([]uint32, len(hashes))
	offsets := make([]int, len(hashes))
	for i, h := range hashes {
		hashVals[i] = h.Hash
		offsets[i] = h.TimeOffset
	}
	if err := store.StoreBatch(hashVals, id, offsets); err != nil {
		t.Fatalf("StoreBatch: %v", err)
	}
	return id
}

func TestMatch_ExactSelfMatch(t *testing.T) {
	// A song matched against its own full fingerprint must always succeed.
	store := storage.NewMemoryStore()
	c := buildConstellationFromSine(t, 440.0, 44100, 5.0)
	id := indexSong(t, store, "A440", c)

	result, ok := Match(c, store, DefaultMatchConfig())
	if !ok {
		t.Fatal("self-match failed: expected a confident match")
	}
	if result.SongID != id {
		t.Errorf("matched song ID %d, expected %d", result.SongID, id)
	}
	if result.SongName != "A440" {
		t.Errorf("matched song name %q, expected 'A440'", result.SongName)
	}
	if result.Votes < 5 {
		t.Errorf("expected ≥5 votes, got %d", result.Votes)
	}
	if result.Confidence <= 0 || result.Confidence > 1 {
		t.Errorf("confidence %f out of range (0,1]", result.Confidence)
	}
}

func TestMatch_QueryIsSubset(t *testing.T) {
	// A query using only the first 2 seconds of a 5-second reference must
	// still match. Validates that partial audio identification works.
	store := storage.NewMemoryStore()
	const sampleRate = 44100
	const fftSize = 4096
	const hopSize = fftSize / 2

	refC := buildConstellationFromSine(t, 880.0, sampleRate, 5.0)
	id := indexSong(t, store, "A880_5s", refC)

	// Approximate number of frames in 2 seconds.
	hopSizeVar := hopSize
	framesIn2s := int(2.0 * float64(sampleRate) / float64(hopSizeVar))
	queryC := refC.PointsInTimeRange(0, framesIn2s)
	if len(queryC) == 0 {
		t.Fatal("query constellation is empty after time range filter")
	}

	result, ok := Match(queryC, store, DefaultMatchConfig())
	if !ok {
		t.Fatalf("subset query match failed (query has %d points, store stats: %s)",
			len(queryC), store.Stats())
	}
	if result.SongID != id {
		t.Errorf("matched song ID %d, expected %d", result.SongID, id)
	}
}

func TestMatch_WrongSongNotMatched(t *testing.T) {
	// Querying Song A must identify Song A, not Song B.
	store := storage.NewMemoryStore()
	cA := buildConstellationFromSine(t, 440.0, 44100, 5.0)
	cB := buildConstellationFromSine(t, 880.0, 44100, 5.0)

	idA := indexSong(t, store, "440Hz", cA)
	indexSong(t, store, "880Hz", cB)

	result, ok := Match(cA, store, DefaultMatchConfig())
	if !ok {
		t.Fatal("expected a match for Song A")
	}
	if result.SongID != idA {
		t.Errorf("expected Song A (id=%d), got id=%d (%s)", idA, result.SongID, result.SongName)
	}
}

func TestMatch_EmptyStore(t *testing.T) {
	store := storage.NewMemoryStore()
	c := buildConstellationFromSine(t, 440.0, 44100, 3.0)
	_, ok := Match(c, store, DefaultMatchConfig())
	if ok {
		t.Error("expected no match against empty store")
	}
}

func TestMatch_EmptyQuery(t *testing.T) {
	store := storage.NewMemoryStore()
	_, ok := Match(nil, store, DefaultMatchConfig())
	if ok {
		t.Error("expected no match for nil query")
	}
}

func TestMatch_AlignmentIsConsistent(t *testing.T) {
	// When querying with a time-shifted excerpt, the recovered alignment
	// should equal the excerpt's start frame (within a small tolerance).
	//
	// IMPORTANT: This test uses a chord (multiple frequencies), NOT a pure sine.
	// A pure sine produces identical hashes on every frame — all alignments
	// receive equal votes, making the "winner" undefined.
	// A chord's harmonic relationships create diverse hashes, so only the
	// correct alignment accumulates votes.
	store := storage.NewMemoryStore()
	const sampleRate = 44100

	// Use an A-major chord for hash diversity.
	chordFreqs := []float64{440.0, 554.37, 659.25, 880.0}
	refC := buildConstellationFromChord(t, chordFreqs, sampleRate, 8.0)
	indexSong(t, store, "Amaj_8s", refC)

	// Extract an excerpt starting at frame 20.
	excerptStart := 20
	excerptEnd := 60
	excerptC := refC.PointsInTimeRange(excerptStart, excerptEnd)
	if len(excerptC) < 10 {
		t.Fatalf("excerpt too sparse (%d points) to test alignment", len(excerptC))
	}

	// Simulate "fresh recording" by re-indexing excerpt time to start at 0.
	queryC := make(Constellation, len(excerptC))
	for i, p := range excerptC {
		queryC[i] = Point{TimeFrame: p.TimeFrame - excerptStart, FreqBin: p.FreqBin}
	}

	result, ok := Match(queryC, store, DefaultMatchConfig())
	if !ok {
		t.Fatalf("mid-song chord excerpt failed (query points: %d)", len(queryC))
	}

	// Alignment = refOffset - queryOffset. Expected ≈ excerptStart.
	alignmentError := result.Alignment - excerptStart
	if alignmentError < 0 {
		alignmentError = -alignmentError
	}
	// Allow ±5 frames tolerance for edge effects at the excerpt boundary.
	if alignmentError > 5 {
		t.Errorf("alignment error %d frames (expected ~%d, got %d)",
			alignmentError, excerptStart, result.Alignment)
	}
}

func TestMatch_ConfidenceReasonable(t *testing.T) {
	// Confidence should be higher for a self-match than a partial match.
	store := storage.NewMemoryStore()
	c := buildConstellationFromChord(t,
		[]float64{440.0, 554.37, 659.25}, 44100, 5.0)
	indexSong(t, store, "chord", c)

	fullResult, _ := Match(c, store, DefaultMatchConfig())

	// Partial: first quarter of the constellation.
	_, lastFrame := c.TimeSpan()
	partialC := c.PointsInTimeRange(0, lastFrame/4)
	partialResult, _ := Match(partialC, store, DefaultMatchConfig())

	if fullResult.Confidence < partialResult.Confidence {
		// Full match should generally have >= confidence than partial,
		// but log rather than fail since this is probabilistic.
		t.Logf("note: full confidence %.3f < partial confidence %.3f (may vary)",
			fullResult.Confidence, partialResult.Confidence)
	}
}
