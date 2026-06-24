// Package api — DSP pipeline wrapper.
//
// Pipeline isolates all DSP configuration from the HTTP handlers.
// Handlers call Pipeline.Index or Pipeline.Query and receive typed results;
// they never touch dsp.SpectrogramConfig or fingerprint.HasherConfig directly.
//
// This boundary makes handlers unit-testable without a real DSP pipeline,
// and makes DSP tuning a single-file concern rather than scattered across handlers.
package api

import (
	"fmt"
	"io"

	"github.com/cheemney/audiofp/audio"
	"github.com/cheemney/audiofp/dsp"
	"github.com/cheemney/audiofp/fingerprint"
	"github.com/cheemney/audiofp/storage"
)

// Pipeline holds the DSP configuration and executes the audio processing chain.
type Pipeline struct {
	spectCfg  dsp.SpectrogramConfig
	peakCfg   dsp.PeakConfig
	hasherCfg fingerprint.HasherConfig
	matchCfg  fingerprint.MatchConfig
}

// IndexResult is returned by Pipeline.Index on success.
type IndexResult struct {
	SongID uint32 `json:"song_id"`
	Name   string `json:"name"`
	Peaks  int    `json:"peaks"`
	Hashes int    `json:"hashes"`
}

// QueryResult is returned by Pipeline.Query on a successful match.
type QueryResult struct {
	SongID     uint32  `json:"song_id"`
	Name       string  `json:"name"`
	Votes      int     `json:"votes"`
	Confidence float64 `json:"confidence"`
	Alignment  int     `json:"alignment_frames"`
}

// NewPipeline constructs a Pipeline with the standard DSP configuration.
func NewPipeline() *Pipeline {
	const (
		fftSize    = 4096
		sampleRate = 44100
	)
	return &Pipeline{
		spectCfg:  dsp.DefaultConfig(),
		peakCfg:   dsp.DefaultPeakConfig(fftSize, sampleRate),
		hasherCfg: fingerprint.DefaultHasherConfig(),
		matchCfg:  fingerprint.DefaultMatchConfig(),
	}
}

// Index reads WAV audio from r, fingerprints it, registers the song under
// name in store, and persists its hashes. Returns an IndexResult on success.
//
// The full pipeline:
//
//	io.Reader → DecodeWAV → Spectrogram → Peaks → Constellation → Hashes → StoreBatch
func (p *Pipeline) Index(r io.Reader, name string, store storage.Store) (IndexResult, error) {
	pcm, err := audio.DecodeWAV(r)
	if err != nil {
		return IndexResult{}, fmt.Errorf("decode wav: %w", err)
	}

	spec, err := dsp.Compute(pcm, p.spectCfg)
	if err != nil {
		return IndexResult{}, fmt.Errorf("spectrogram: %w", err)
	}

	peaks := dsp.ExtractPeaks(spec, p.peakCfg)
	constel := fingerprint.FromPeaks(peaks)
	hashes := fingerprint.GenerateHashes(constel, p.hasherCfg)

	songID, err := store.RegisterSong(name)
	if err != nil {
		return IndexResult{}, fmt.Errorf("register song: %w", err)
	}

	hashVals := make([]uint32, len(hashes))
	offsets := make([]int, len(hashes))
	for i, h := range hashes {
		hashVals[i] = h.Hash
		offsets[i] = h.TimeOffset
	}
	if err := store.StoreBatch(hashVals, songID, offsets); err != nil {
		return IndexResult{}, fmt.Errorf("store fingerprints: %w", err)
	}

	return IndexResult{
		SongID: songID,
		Name:   name,
		Peaks:  len(peaks),
		Hashes: len(hashes),
	}, nil
}

// Query reads WAV audio from r, fingerprints it, and attempts to match it
// against the store. Returns (result, true) on a confident match.
//
// The full pipeline:
//
//	io.Reader → DecodeWAV → Spectrogram → Peaks → Constellation → Hashes → Match
func (p *Pipeline) Query(r io.Reader, store storage.Store) (QueryResult, bool, error) {
	pcm, err := audio.DecodeWAV(r)
	if err != nil {
		return QueryResult{}, false, fmt.Errorf("decode wav: %w", err)
	}

	spec, err := dsp.Compute(pcm, p.spectCfg)
	if err != nil {
		return QueryResult{}, false, fmt.Errorf("spectrogram: %w", err)
	}

	peaks := dsp.ExtractPeaks(spec, p.peakCfg)
	constel := fingerprint.FromPeaks(peaks)

	result, ok := fingerprint.Match(constel, store, p.matchCfg)
	if !ok {
		return QueryResult{}, false, nil
	}

	return QueryResult{
		SongID:     result.SongID,
		Name:       result.SongName,
		Votes:      result.Votes,
		Confidence: result.Confidence,
		Alignment:  result.Alignment,
	}, true, nil
}
