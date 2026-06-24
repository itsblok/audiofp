// Package fingerprint — time-alignment voting matcher.
//
// This is Shazam's core statistical mechanism. The insight:
//
//	If you query with a 10-second clip from minute 2 of a song, every hash
//	that matches will have: refOffset - queryOffset = constant (≈120 frames).
//	This constant is the time alignment. Genuine matches cluster; random
//	collisions are scattered across all possible alignments.
//
//	So matching is a voting problem: which (songID, alignment) pair gets the
//	most votes? The winner with enough votes is the match.
package fingerprint

import (
	"github.com/cheemney/audiofp/storage"
)

// MatchResult holds the output of a successful match.
type MatchResult struct {
	SongID     uint32
	SongName   string
	Votes      int     // how many hashes agreed on this (songID, alignment)
	TotalHits  int     // total hashes that found any posting in the database
	Confidence float64 // Votes / TotalHits — fraction of hits that agreed
	Alignment  int     // recovered time offset: refFrame - queryFrame
}

// MatchConfig controls the matching algorithm.
type MatchConfig struct {
	// HasherCfg is used to generate hashes from the query constellation.
	// Must match the config used during indexing.
	HasherCfg HasherConfig

	// MinVotes is the minimum number of aligned votes required to declare
	// a match. Too low → false positives; too high → missed matches.
	MinVotes int

	// MinConfidence is the minimum Votes/TotalHits ratio required.
	// Filters out cases where many hashes match but they disagree on alignment.
	MinConfidence float64
}

// DefaultMatchConfig returns safe defaults for the matching step.
func DefaultMatchConfig() MatchConfig {
	return MatchConfig{
		HasherCfg:     DefaultHasherConfig(),
		MinVotes:      5,
		MinConfidence: 0.01, // 1% of hits must agree — permissive for research
	}
}

// alignmentKey uniquely identifies a (songID, timeAlignment) hypothesis.
// A high vote count for one key means "this song, starting at this offset".
type alignmentKey struct {
	songID    uint32
	alignment int // refOffset - queryOffset
}

// Match attempts to identify query audio against the fingerprint database.
//
// Returns (result, true) on a confident match, or (zero, false) otherwise.
//
// Algorithm:
//  1. Generate hashes from the query constellation (same fan-out as indexing).
//  2. For each query hash H with anchor at queryOffset:
//     For each database posting (songID, refOffset) matching H:
//     alignment = refOffset - queryOffset
//     votes[songID][alignment]++
//  3. Find the (songID, alignment) key with the maximum vote count.
//  4. If maxVotes >= MinVotes AND confidence >= MinConfidence → match found.
func Match(query Constellation, store storage.Store, cfg MatchConfig) (MatchResult, bool) {
	// Generate hashes from the query the same way as during indexing.
	queryHashes := GenerateHashes(query, cfg.HasherCfg)
	if len(queryHashes) == 0 {
		return MatchResult{}, false
	}

	votes := make(map[alignmentKey]int)
	totalHits := 0

	for _, qh := range queryHashes {
		postings, err := store.Lookup(qh.Hash)
		if err != nil || len(postings) == 0 {
			continue
		}

		for _, p := range postings {
			// Recover the time alignment hypothesis for this hit.
			// If this is a true match: refOffset - queryOffset = constant
			// across ALL matching hashes (they shift together in time).
			alignment := p.TimeOffset - qh.TimeOffset
			votes[alignmentKey{p.SongID, alignment}]++
			totalHits++
		}
	}

	if totalHits == 0 {
		return MatchResult{}, false
	}

	// Find the (songID, alignment) with the most votes.
	var bestKey alignmentKey
	bestVotes := 0
	for key, count := range votes {
		if count > bestVotes {
			bestVotes = count
			bestKey = key
		}
	}

	confidence := float64(bestVotes) / float64(totalHits)

	if bestVotes < cfg.MinVotes || confidence < cfg.MinConfidence {
		return MatchResult{}, false
	}

	name, _ := store.SongName(bestKey.songID)
	return MatchResult{
		SongID:     bestKey.songID,
		SongName:   name,
		Votes:      bestVotes,
		TotalHits:  totalHits,
		Confidence: confidence,
		Alignment:  bestKey.alignment,
	}, true
}
