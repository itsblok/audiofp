package dsp

import "testing"

func TestLogBands_Count(t *testing.T) {
	bs := LogBands(6, 300.0, 10000.0, 4096, 44100)
	if len(bs.Bands) != 6 {
		t.Errorf("expected 6 bands, got %d", len(bs.Bands))
	}
}

func TestLogBands_BinLo_LessThan_BinHi(t *testing.T) {
	bs := LogBands(6, 300.0, 10000.0, 4096, 44100)
	for _, b := range bs.Bands {
		if b.BinLo > b.BinHi {
			t.Errorf("band %d: BinLo %d > BinHi %d", b.Index, b.BinLo, b.BinHi)
		}
	}
}

func TestLogBands_FreqLo_LessThan_FreqHi(t *testing.T) {
	bs := LogBands(6, 300.0, 10000.0, 4096, 44100)
	for _, b := range bs.Bands {
		if b.FreqLo >= b.FreqHi {
			t.Errorf("band %d: FreqLo %.1f >= FreqHi %.1f", b.Index, b.FreqLo, b.FreqHi)
		}
	}
}

func TestLogBands_BandOf_InRange(t *testing.T) {
	bs := DefaultBands(4096, 44100)
	fftSize, sampleRate := 4096, 44100
	bin1kHz := int(1000.0 * float64(fftSize) / float64(sampleRate))
	band := bs.BandOf(bin1kHz)
	if band < 0 {
		t.Errorf("1kHz bin %d not assigned to any band", bin1kHz)
	}
}

func TestLogBands_BandOf_OutOfRange(t *testing.T) {
	bs := DefaultBands(4096, 44100)
	if bs.BandOf(0) != -1 {
		t.Errorf("bin 0 (DC) should not be in any band")
	}
	if bs.BandOf(99999) != -1 {
		t.Errorf("out-of-bounds bin should return -1")
	}
}

func TestLogBands_Monotone(t *testing.T) {
	bs := LogBands(6, 300.0, 10000.0, 4096, 44100)
	for i := 0; i < len(bs.Bands)-1; i++ {
		if bs.Bands[i].BinHi > bs.Bands[i+1].BinLo {
			t.Errorf("band %d BinHi %d overlaps band %d BinLo %d",
				i, bs.Bands[i].BinHi, i+1, bs.Bands[i+1].BinLo)
		}
	}
}
