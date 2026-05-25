// Package storage defines the fingerprint database interface and types.
//
// The Store interface is defined entirely in terms of primitive types —
// no imports from the fingerprint package. This avoids an import cycle
// since fingerprint/matcher.go needs to import storage for matching.
//
// Concrete implementations:
//
//	MemoryStore  — map-backed, for development and unit tests (this file)
//	SQLiteStore  — persistent, for production use (Step 4)
package storage

import (
	"errors"
	"fmt"
	"sync"
)

// Posting is a single entry in the inverted index.
// Records that a given hash was produced by SongID at anchor TimeOffset.
type Posting struct {
	SongID     uint32
	TimeOffset int // anchor frame index in the reference recording
}

// ErrSongNotFound is returned when a song ID is not registered.
var ErrSongNotFound = errors.New("storage: song ID not found")

// ErrDuplicateSong is returned when registering an already-registered name.
var ErrDuplicateSong = errors.New("storage: song name already registered")

// Store is the fingerprint database interface.
// All methods must be safe for concurrent use.
type Store interface {
	// RegisterSong registers a song by name and returns its assigned ID.
	RegisterSong(name string) (uint32, error)

	// SongName returns the name of a registered song by ID.
	SongName(id uint32) (string, bool)

	// StoreHash records a single (hash, songID, timeOffset) triple.
	StoreHash(hash uint32, songID uint32, timeOffset int) error

	// StoreBatch records many hashes for one song efficiently.
	// hashes and offsets must have equal length.
	StoreBatch(hashes []uint32, songID uint32, offsets []int) error

	// Lookup returns all postings for a given hash value.
	// Returns an empty slice (not an error) when no matches exist.
	Lookup(hash uint32) ([]Posting, error)

	// Stats returns diagnostic counts about stored data.
	Stats() StoreStats
}

// StoreStats holds diagnostic counts about a Store.
type StoreStats struct {
	Songs         int
	HashEntries   int // number of distinct hash values
	TotalPostings int // total number of (hash, songID, offset) records
}

func (s StoreStats) String() string {
	return fmt.Sprintf("StoreStats{songs=%d hashEntries=%d totalPostings=%d}",
		s.Songs, s.HashEntries, s.TotalPostings)
}

// MemoryStore is a thread-safe in-memory Store backed by Go maps.
// Suitable for development, testing, and small datasets.
type MemoryStore struct {
	mu       sync.RWMutex
	index    map[uint32][]Posting // hash → postings
	songs    map[uint32]string    // id → name
	nameToID map[string]uint32    // name → id (for duplicate detection)
	nextID   uint32
}

// NewMemoryStore creates an empty MemoryStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		index:    make(map[uint32][]Posting),
		songs:    make(map[uint32]string),
		nameToID: make(map[string]uint32),
		nextID:   1, // 0 is reserved as the invalid/unset ID
	}
}

func (m *MemoryStore) RegisterSong(name string) (uint32, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.nameToID[name]; exists {
		return 0, ErrDuplicateSong
	}
	id := m.nextID
	m.nextID++
	m.songs[id] = name
	m.nameToID[name] = id
	return id, nil
}

func (m *MemoryStore) SongName(id uint32) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	name, ok := m.songs[id]
	return name, ok
}

func (m *MemoryStore) StoreHash(hash uint32, songID uint32, timeOffset int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.index[hash] = append(m.index[hash], Posting{SongID: songID, TimeOffset: timeOffset})
	return nil
}

func (m *MemoryStore) StoreBatch(hashes []uint32, songID uint32, offsets []int) error {
	if len(hashes) != len(offsets) {
		return fmt.Errorf("storage: hashes length %d != offsets length %d", len(hashes), len(offsets))
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, h := range hashes {
		m.index[h] = append(m.index[h], Posting{SongID: songID, TimeOffset: offsets[i]})
	}
	return nil
}

func (m *MemoryStore) Lookup(hash uint32) ([]Posting, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	postings := m.index[hash]
	if len(postings) == 0 {
		return nil, nil
	}
	// Return a copy to prevent data races if the caller holds the slice.
	result := make([]Posting, len(postings))
	copy(result, postings)
	return result, nil
}

func (m *MemoryStore) Stats() StoreStats {
	m.mu.RLock()
	defer m.mu.RUnlock()
	total := 0
	for _, postings := range m.index {
		total += len(postings)
	}
	return StoreStats{
		Songs:         len(m.songs),
		HashEntries:   len(m.index),
		TotalPostings: total,
	}
}
