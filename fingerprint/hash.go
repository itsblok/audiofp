// Package fingerprint — hash record type and bit-packing.
//
// A fingerprint hash encodes the relationship between two constellation points
// (an "anchor" and a "target") into a single uint32. The hash is deliberately
// compact so that millions of records can be held in memory or a B-tree index.
//
// Bit layout:
//
//	[31..22]  anchor FreqBin  (10 bits, range 0–1023)
//	[21..12]  target FreqBin  (10 bits, range 0–1023)
//	[11.. 0]  delta time      (12 bits, range 0–4095 frames ≈ 190s @ 46ms/frame)
//
// Why these three fields?
//
//	Two frequencies: captures the *relationship* between spectral peaks.
//	Two simultaneous notes are far more unique than either alone.
//
//	Delta time: encodes temporal structure. A drum hit 200ms before a chord
//	creates a specific pattern unlikely to recur by coincidence.
//
//	No absolute time: the hash is position-invariant. The same phrase at
//	second 10 or second 60 produces identical hashes — so matching works
//	regardless of where in the song the query audio starts.
package fingerprint

import "fmt"

// HashRecord is the output of the fan-out pairing step.
// It ties a packed hash to the anchor's position in the source audio.
// TimeOffset is used during matching to recover the time alignment.
type HashRecord struct {
	Hash       uint32
	TimeOffset int // anchor's TimeFrame index in the source audio
}

// Bit widths for hash packing.
const (
	deltaTimeBits  = 12
	freqBinBits    = 10
	maxDeltaFrames = (1 << deltaTimeBits) - 1 // 4095
	maxFreqBin     = (1 << freqBinBits) - 1   // 1023
)

// PackHash encodes (anchorBin, targetBin, deltaFrames) into a uint32.
// Values exceeding their bit width are clamped to prevent overflow.
func PackHash(anchorBin, targetBin, deltaFrames int) uint32 {
	if anchorBin > maxFreqBin {
		anchorBin = maxFreqBin
	}
	if targetBin > maxFreqBin {
		targetBin = maxFreqBin
	}
	if deltaFrames > maxDeltaFrames {
		deltaFrames = maxDeltaFrames
	}
	if anchorBin < 0 {
		anchorBin = 0
	}
	if targetBin < 0 {
		targetBin = 0
	}
	if deltaFrames < 0 {
		deltaFrames = 0
	}

	return uint32(anchorBin<<(freqBinBits+deltaTimeBits)) |
		uint32(targetBin<<deltaTimeBits) |
		uint32(deltaFrames)
}

// UnpackHash reverses PackHash into its three components.
// Useful for debugging and assertions.
func UnpackHash(h uint32) (anchorBin, targetBin, deltaFrames int) {
	deltaFrames = int(h & maxDeltaFrames)
	targetBin = int((h >> deltaTimeBits) & maxFreqBin)
	anchorBin = int((h >> (deltaTimeBits + freqBinBits)) & maxFreqBin)
	return
}

// String returns a human-readable representation for debugging.
func (r HashRecord) String() string {
	a, t, d := UnpackHash(r.Hash)
	return fmt.Sprintf("HashRecord{hash=0x%08X anchor=%d target=%d delta=%d offset=%d}",
		r.Hash, a, t, d, r.TimeOffset)
}
