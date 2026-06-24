package fingerprint

import (
	"testing"

	"github.com/cheemney/audiofp/dsp"
)

func TestFromPeaks_Length(t *testing.T) {
	peaks := []dsp.Peak{
		{TimeFrame: 0, FreqBin: 10, MagnitudeDB: -20},
		{TimeFrame: 1, FreqBin: 20, MagnitudeDB: -15},
		{TimeFrame: 2, FreqBin: 30, MagnitudeDB: -10},
	}
	c := FromPeaks(peaks)
	if c.Len() != 3 {
		t.Errorf("expected 3 points, got %d", c.Len())
	}
}

func TestFromPeaks_MagnitudeDiscarded(t *testing.T) {
	// Ensure MagnitudeDB is not carried into the constellation.
	peaks := []dsp.Peak{
		{TimeFrame: 5, FreqBin: 42, MagnitudeDB: -99},
	}
	c := FromPeaks(peaks)
	if c[0].TimeFrame != 5 || c[0].FreqBin != 42 {
		t.Errorf("wrong point: got %+v", c[0])
	}
}

func TestTimeSpan(t *testing.T) {
	c := Constellation{
		{TimeFrame: 3, FreqBin: 10},
		{TimeFrame: 1, FreqBin: 20},
		{TimeFrame: 7, FreqBin: 5},
	}
	first, last := c.TimeSpan()
	if first != 1 || last != 7 {
		t.Errorf("TimeSpan: expected (1,7), got (%d,%d)", first, last)
	}
}

func TestTimeSpan_Empty(t *testing.T) {
	var c Constellation
	first, last := c.TimeSpan()
	if first != 0 || last != 0 {
		t.Errorf("empty TimeSpan should be (0,0), got (%d,%d)", first, last)
	}
}

func TestPointsInTimeRange(t *testing.T) {
	c := Constellation{
		{TimeFrame: 1, FreqBin: 10},
		{TimeFrame: 3, FreqBin: 20},
		{TimeFrame: 5, FreqBin: 30},
		{TimeFrame: 7, FreqBin: 40},
	}
	sub := c.PointsInTimeRange(3, 5)
	if len(sub) != 2 {
		t.Errorf("expected 2 points in [3,5], got %d", len(sub))
	}
}

func TestSortedByTime(t *testing.T) {
	c := Constellation{
		{TimeFrame: 5, FreqBin: 1},
		{TimeFrame: 2, FreqBin: 3},
		{TimeFrame: 2, FreqBin: 1},
	}
	sorted := c.SortedByTime()
	if sorted[0].TimeFrame != 2 || sorted[0].FreqBin != 1 {
		t.Errorf("wrong first element: %+v", sorted[0])
	}
	if sorted[1].FreqBin != 3 {
		t.Errorf("wrong second element: %+v", sorted[1])
	}
	if sorted[2].TimeFrame != 5 {
		t.Errorf("wrong third element: %+v", sorted[2])
	}
}

func TestBandDistribution(t *testing.T) {
	c := Constellation{
		{TimeFrame: 0, FreqBin: 10},
		{TimeFrame: 0, FreqBin: 10},
		{TimeFrame: 1, FreqBin: 20},
	}
	// bandOf maps bin 10 → band 0, bin 20 → band 1
	bandOf := func(bin int) int {
		if bin == 10 {
			return 0
		}
		if bin == 20 {
			return 1
		}
		return -1
	}
	dist := c.BandDistribution(bandOf)
	if dist[0] != 2 {
		t.Errorf("band 0: expected 2, got %d", dist[0])
	}
	if dist[1] != 1 {
		t.Errorf("band 1: expected 1, got %d", dist[1])
	}
}
