// Package fingerprint — fan-out hash generation from a constellation map.
package fingerprint

// HasherConfig controls the fan-out pairing algorithm.
type HasherConfig struct {
	// FanOutWindow is the maximum delta-time (in frames) between an anchor
	// and its target points. Larger window = more hashes per anchor = better
	// recall, but more storage and slower matching.
	//
	// 100 frames × 46ms/frame ≈ 4.6 seconds of lookahead.
	// This is long enough to capture slow melodic patterns.
	FanOutWindow int

	// MaxTargetsPerAnchor caps the number of target pairings per anchor.
	// Without this cap, dense constellations produce O(N²) hashes.
	// 5 targets per anchor is the Shazam-reported sweet spot.
	MaxTargetsPerAnchor int
}

// DefaultHasherConfig returns the standard configuration.
func DefaultHasherConfig() HasherConfig {
	return HasherConfig{
		FanOutWindow:        100,
		MaxTargetsPerAnchor: 5,
	}
}

// GenerateHashes produces all fingerprint hash records from a constellation.
//
// The fan-out algorithm:
//
//	For each anchor point A (in time order):
//	  Scan forward through points T where:
//	    1. T.TimeFrame > A.TimeFrame  (target must come after anchor)
//	    2. T.TimeFrame - A.TimeFrame <= FanOutWindow
//	  For up to MaxTargetsPerAnchor such T:
//	    hash    = PackHash(A.FreqBin, T.FreqBin, T.TimeFrame - A.TimeFrame)
//	    record  = HashRecord{Hash: hash, TimeOffset: A.TimeFrame}
//
// The TimeOffset stored is A.TimeFrame — the anchor's position in the audio.
// During matching, subtracting the query anchor's time from this offset
// gives the time alignment between query and reference.
//
// Note: GenerateHashes is songID-agnostic. SongID is assigned by the storage
// layer when a song is registered. This keeps hash generation pure/testable.
func GenerateHashes(c Constellation, cfg HasherConfig) []HashRecord {
	if len(c) == 0 {
		return nil
	}

	// Constellation must be sorted by time for the forward scan to work.
	sorted := c.SortedByTime()

	var records []HashRecord

	for i := 0; i < len(sorted); i++ {
		anchor := sorted[i]
		targetsFound := 0

		// Forward scan: only look at points that come after the anchor in time.
		for j := i + 1; j < len(sorted); j++ {
			target := sorted[j]
			delta := target.TimeFrame - anchor.TimeFrame

			// Stop scanning once we exceed the lookahead window.
			if delta > cfg.FanOutWindow {
				break
			}

			// Skip same-time points (delta == 0 would be ambiguous).
			if delta <= 0 {
				continue
			}

			hash := PackHash(anchor.FreqBin, target.FreqBin, delta)
			records = append(records, HashRecord{
				Hash:       hash,
				TimeOffset: anchor.TimeFrame,
			})

			targetsFound++
			if targetsFound >= cfg.MaxTargetsPerAnchor {
				break
			}
		}
	}

	return records
}
