package fingerprint

import "testing"

func makeTestConstellation() Constellation {
	// A simple 10-point constellation spread across time.
	return Constellation{
		{TimeFrame: 0, FreqBin: 41},
		{TimeFrame: 0, FreqBin: 92},
		{TimeFrame: 2, FreqBin: 55},
		{TimeFrame: 5, FreqBin: 80},
		{TimeFrame: 5, FreqBin: 120},
		{TimeFrame: 10, FreqBin: 41},
		{TimeFrame: 15, FreqBin: 200},
		{TimeFrame: 20, FreqBin: 41},
		{TimeFrame: 25, FreqBin: 60},
		{TimeFrame: 30, FreqBin: 90},
	}
}

func TestGenerateHashes_NonEmpty(t *testing.T) {
	c := makeTestConstellation()
	cfg := DefaultHasherConfig()
	hashes := GenerateHashes(c, cfg)
	if len(hashes) == 0 {
		t.Error("expected hashes from a non-empty constellation")
	}
}

func TestGenerateHashes_EmptyConstellation(t *testing.T) {
	hashes := GenerateHashes(nil, DefaultHasherConfig())
	if len(hashes) != 0 {
		t.Errorf("expected 0 hashes from nil constellation, got %d", len(hashes))
	}
}

func TestGenerateHashes_MaxTargetsRespected(t *testing.T) {
	// A single anchor should produce at most MaxTargetsPerAnchor hashes.
	// Place the anchor at t=0 and many targets within the window.
	c := Constellation{{TimeFrame: 0, FreqBin: 50}}
	for i := 1; i <= 20; i++ {
		c = append(c, Point{TimeFrame: i, FreqBin: 50 + i})
	}

	cfg := HasherConfig{FanOutWindow: 100, MaxTargetsPerAnchor: 5}
	hashes := GenerateHashes(c, cfg)

	// First anchor (t=0) can produce at most 5 hashes.
	// Each subsequent anchor can also produce up to 5, but we just check
	// that we never exceed 5 per anchor.
	anchorCounts := make(map[int]int)
	for _, h := range hashes {
		anchorCounts[h.TimeOffset]++
	}
	for offset, count := range anchorCounts {
		if count > cfg.MaxTargetsPerAnchor {
			t.Errorf("anchor at offset %d produced %d hashes, max is %d",
				offset, count, cfg.MaxTargetsPerAnchor)
		}
	}
}

func TestGenerateHashes_FanOutWindowRespected(t *testing.T) {
	// Targets beyond FanOutWindow must not be paired with the anchor.
	c := Constellation{
		{TimeFrame: 0, FreqBin: 41},  // anchor
		{TimeFrame: 10, FreqBin: 80}, // inside window
		{TimeFrame: 50, FreqBin: 90}, // outside a window of 20
	}
	cfg := HasherConfig{FanOutWindow: 20, MaxTargetsPerAnchor: 10}
	hashes := GenerateHashes(c, cfg)

	// Verify no hash encodes a delta > FanOutWindow.
	for _, h := range hashes {
		_, _, delta := UnpackHash(h.Hash)
		if delta > cfg.FanOutWindow {
			t.Errorf("hash has delta %d > FanOutWindow %d", delta, cfg.FanOutWindow)
		}
	}
}

func TestGenerateHashes_TimeOffsetIsAnchor(t *testing.T) {
	// TimeOffset in the HashRecord must be the anchor's TimeFrame, not target's.
	c := Constellation{
		{TimeFrame: 5, FreqBin: 41},
		{TimeFrame: 10, FreqBin: 80},
	}
	cfg := HasherConfig{FanOutWindow: 100, MaxTargetsPerAnchor: 5}
	hashes := GenerateHashes(c, cfg)

	if len(hashes) == 0 {
		t.Fatal("expected at least one hash")
	}
	// The only possible anchor is t=5 (t=10 has no forward targets).
	for _, h := range hashes {
		if h.TimeOffset != 5 {
			t.Errorf("expected TimeOffset=5 (anchor), got %d", h.TimeOffset)
		}
	}
}

func TestGenerateHashes_DeltaEncoded(t *testing.T) {
	// The delta in the hash must equal target.TimeFrame - anchor.TimeFrame.
	c := Constellation{
		{TimeFrame: 3, FreqBin: 41},
		{TimeFrame: 8, FreqBin: 80}, // delta = 5
	}
	cfg := HasherConfig{FanOutWindow: 100, MaxTargetsPerAnchor: 5}
	hashes := GenerateHashes(c, cfg)

	if len(hashes) != 1 {
		t.Fatalf("expected exactly 1 hash, got %d", len(hashes))
	}
	_, _, delta := UnpackHash(hashes[0].Hash)
	if delta != 5 {
		t.Errorf("expected delta=5, got %d", delta)
	}
}
