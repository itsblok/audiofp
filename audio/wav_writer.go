// Package audio — RIFF/WAV encoder.
//
// EncodeWAV writes to any io.Writer, making it usable for HTTP response bodies,
// multipart uploads, in-memory buffers, and os.File alike.
// WriteWAV is a convenience wrapper for the common file-on-disk case.
package audio

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
)

// EncodeWAV encodes a PCM signal as 16-bit signed mono WAV and writes it to w.
// Samples are clipped to [-1.0, 1.0] before conversion to int16.
func EncodeWAV(w io.Writer, pcm PCM) error {
	nSamples := len(pcm.Samples)
	dataSize := uint32(nSamples * 2) // 2 bytes per 16-bit sample
	// Total file = RIFF(12) + fmt chunk(8+16) + data chunk(8) + data
	chunkSize := uint32(36) + dataSize

	wr := &errWriter{w: w}

	// RIFF container header
	wr.write([]byte("RIFF"))
	wr.writeU32(chunkSize) // total file size − 8 (the RIFF tag + this field)
	wr.write([]byte("WAVE"))

	// fmt sub-chunk (16 bytes of PCM descriptor)
	wr.write([]byte("fmt "))
	wr.writeU32(16)               // sub-chunk size for PCM
	wr.writeU16(wavFormatPCM)     // audio format: PCM = 1
	wr.writeU16(1)                // mono
	wr.writeU32(uint32(pcm.SampleRate))
	wr.writeU32(uint32(pcm.SampleRate * 2)) // byte rate = sampleRate × blockAlign
	wr.writeU16(2)                           // block align = channels × bytesPerSample
	wr.writeU16(16)                          // bits per sample

	// data sub-chunk
	wr.write([]byte("data"))
	wr.writeU32(dataSize)

	// Sample data: float64 → int16 little-endian
	var buf [2]byte
	for _, s := range pcm.Samples {
		if s > 1.0 {
			s = 1.0
		} else if s < -1.0 {
			s = -1.0
		}
		val := int16(math.Round(s * float64(math.MaxInt16)))
		binary.LittleEndian.PutUint16(buf[:], uint16(val))
		wr.write(buf[:])
	}

	return wr.err
}

// WriteWAV encodes pcm as a WAV file at the given path.
// A convenience wrapper around EncodeWAV for the common file-on-disk case.
func WriteWAV(path string, pcm PCM) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("wav: create %q: %w", path, err)
	}
	defer f.Close()
	if err := EncodeWAV(f, pcm); err != nil {
		return fmt.Errorf("wav: encode to %q: %w", path, err)
	}
	return nil
}

// errWriter accumulates the first write error and silently drops subsequent
// writes. This avoids nested error checks in the header-writing sequence.
type errWriter struct {
	w   io.Writer
	err error
}

func (e *errWriter) write(b []byte) {
	if e.err != nil {
		return
	}
	_, e.err = e.w.Write(b)
}

func (e *errWriter) writeU32(v uint32) {
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], v)
	e.write(buf[:])
}

func (e *errWriter) writeU16(v uint16) {
	var buf [2]byte
	binary.LittleEndian.PutUint16(buf[:], v)
	e.write(buf[:])
}
