// Package audio — RIFF/WAV file reader.
//
// WAV is a chunked format: a RIFF container holding a "WAVE" type, with
// two mandatory chunks: "fmt " (format descriptor) and "data" (raw PCM).
// Additional chunks (LIST, INFO, smpl, ...) may appear in any order and
// must be skipped gracefully.
//
// Supported PCM formats:
//   8-bit  unsigned (uint8,  offset-binary: 0=silence, 128=max)
//   16-bit signed   (int16,  little-endian — the most common WAV format)
//   32-bit signed   (int32,  little-endian)
//
// All are converted to float64 normalized to [-1.0, 1.0].
// Stereo is downmixed to mono by averaging L+R channels.
package audio

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
)

// WAV format codes (PCM = 1, IEEE float = 3).
const (
	wavFormatPCM       = 1
	wavFormatIEEEFloat = 3
)

// WAV-specific errors.
var (
	ErrNotWAV           = errors.New("wav: not a RIFF/WAVE file")
	ErrNoFmtChunk       = errors.New("wav: missing fmt chunk")
	ErrNoDataChunk      = errors.New("wav: missing data chunk")
	ErrUnsupportedFormat = errors.New("wav: unsupported audio format (only PCM supported)")
	ErrUnsupportedDepth  = errors.New("wav: unsupported bit depth (only 8/16/32-bit supported)")
)

// riffHeader is the 12-byte RIFF file header.
type riffHeader struct {
	ChunkID   [4]byte // "RIFF"
	ChunkSize uint32  // file size − 8
	Format    [4]byte // "WAVE"
}

// fmtChunk is the content of the "fmt " chunk (at least 16 bytes for PCM).
type fmtChunk struct {
	AudioFormat   uint16 // 1 = PCM
	NumChannels   uint16
	SampleRate    uint32
	ByteRate      uint32
	BlockAlign    uint16
	BitsPerSample uint16
}

// ReadWAV reads a WAV file from disk and returns a mono float64 PCM signal.
func ReadWAV(path string) (PCM, error) {
	f, err := os.Open(path)
	if err != nil {
		return PCM{}, fmt.Errorf("wav: open %q: %w", path, err)
	}
	defer f.Close()
	return DecodeWAV(f)
}

// DecodeWAV reads a WAV stream from any io.Reader.
// Useful for reading from memory buffers in tests.
func DecodeWAV(r io.Reader) (PCM, error) {
	// --- Parse RIFF header ---
	var hdr riffHeader
	if err := binary.Read(r, binary.LittleEndian, &hdr); err != nil {
		return PCM{}, ErrNotWAV
	}
	if string(hdr.ChunkID[:]) != "RIFF" || string(hdr.Format[:]) != "WAVE" {
		return PCM{}, ErrNotWAV
	}

	// --- Scan chunks until we find fmt and data ---
	var fmt_ fmtChunk
	var rawSamples []byte
	hasFmt := false

	for {
		// Read 8-byte chunk header: 4-byte ID + 4-byte size.
		var chunkID [4]byte
		var chunkSize uint32
		if err := binary.Read(r, binary.LittleEndian, &chunkID); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return PCM{}, fmt.Errorf("wav: reading chunk ID: %w", err)
		}
		if err := binary.Read(r, binary.LittleEndian, &chunkSize); err != nil {
			return PCM{}, fmt.Errorf("wav: reading chunk size: %w", err)
		}

		id := string(chunkID[:])

		switch id {
		case "fmt ":
			// fmt chunk must be at least 16 bytes (standard PCM header).
			// Some encoders write 18 or 40 bytes — read first 16, skip rest.
			if chunkSize < 16 {
				return PCM{}, ErrNoFmtChunk
			}
			if err := binary.Read(r, binary.LittleEndian, &fmt_); err != nil {
				return PCM{}, fmt.Errorf("wav: reading fmt chunk: %w", err)
			}
			// Skip any extra bytes in an extended fmt chunk.
			if extra := int(chunkSize) - 16; extra > 0 {
				io.CopyN(io.Discard, r, int64(extra))
			}
			hasFmt = true

		case "data":
			// data chunk contains the raw PCM samples.
			rawSamples = make([]byte, chunkSize)
			if _, err := io.ReadFull(r, rawSamples); err != nil {
				return PCM{}, fmt.Errorf("wav: reading data chunk: %w", err)
			}

		default:
			// Unknown chunk (LIST, INFO, smpl, etc.) — skip it entirely.
			// Chunk sizes are always rounded up to an even byte count.
			skipSize := int64(chunkSize)
			if skipSize%2 != 0 {
				skipSize++
			}
			io.CopyN(io.Discard, r, skipSize)
		}
	}

	if !hasFmt {
		return PCM{}, ErrNoFmtChunk
	}
	if rawSamples == nil {
		return PCM{}, ErrNoDataChunk
	}
	if fmt_.AudioFormat != wavFormatPCM {
		return PCM{}, ErrUnsupportedFormat
	}

	// --- Convert raw bytes to float64 samples ---
	samples, err := decodeSamples(rawSamples, fmt_)
	if err != nil {
		return PCM{}, err
	}

	return PCM{
		Samples:    samples,
		SampleRate: int(fmt_.SampleRate),
	}, nil
}

// decodeSamples converts raw PCM bytes to normalized float64 mono samples.
// Handles 8, 16, and 32-bit depths, and downmixes stereo to mono.
func decodeSamples(raw []byte, fmt_ fmtChunk) ([]float64, error) {
	bytesPerSample := int(fmt_.BitsPerSample / 8)
	nChannels := int(fmt_.NumChannels)

	if bytesPerSample == 0 || nChannels == 0 {
		return nil, fmt.Errorf("wav: invalid format: %d bits, %d channels",
			fmt_.BitsPerSample, nChannels)
	}

	bytesPerFrame := bytesPerSample * nChannels
	nFrames := len(raw) / bytesPerFrame
	if nFrames == 0 {
		return nil, ErrEmptyAudio
	}

	samples := make([]float64, nFrames)

	for i := 0; i < nFrames; i++ {
		frameStart := i * bytesPerFrame
		var mono float64

		// Sum all channels for this frame, then divide for mono average.
		for ch := 0; ch < nChannels; ch++ {
			sampleStart := frameStart + ch*bytesPerSample
			var val float64

			switch fmt_.BitsPerSample {
			case 8:
				// 8-bit WAV is unsigned: 0=min, 128=silence, 255=max.
				// Normalize to [-1, 1] by subtracting 128 and dividing by 128.
				val = (float64(raw[sampleStart]) - 128.0) / 128.0

			case 16:
				// 16-bit is signed little-endian int16.
				s := int16(binary.LittleEndian.Uint16(raw[sampleStart:]))
				val = float64(s) / float64(math.MaxInt16)

			case 32:
				// 32-bit is signed little-endian int32.
				s := int32(binary.LittleEndian.Uint32(raw[sampleStart:]))
				val = float64(s) / float64(math.MaxInt32)

			default:
				return nil, ErrUnsupportedDepth
			}

			mono += val
		}

		samples[i] = mono / float64(nChannels)
	}

	return samples, nil
}
