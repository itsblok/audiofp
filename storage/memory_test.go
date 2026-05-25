package storage

import (
	"sync"
	"testing"
)

func TestMemoryStore_RegisterAndLookup(t *testing.T) {
	s := NewMemoryStore()
	id, err := s.RegisterSong("Test Song")
	if err != nil {
		t.Fatalf("RegisterSong: %v", err)
	}
	if id == 0 {
		t.Error("expected non-zero song ID")
	}
	name, ok := s.SongName(id)
	if !ok || name != "Test Song" {
		t.Errorf("SongName(%d): expected 'Test Song', got %q, ok=%v", id, name, ok)
	}
}

func TestMemoryStore_DuplicateSong(t *testing.T) {
	s := NewMemoryStore()
	s.RegisterSong("Song A")
	_, err := s.RegisterSong("Song A")
	if err != ErrDuplicateSong {
		t.Errorf("expected ErrDuplicateSong, got %v", err)
	}
}

func TestMemoryStore_SongNameNotFound(t *testing.T) {
	s := NewMemoryStore()
	_, ok := s.SongName(999)
	if ok {
		t.Error("expected ok=false for unknown song ID")
	}
}

func TestMemoryStore_StoreAndLookup(t *testing.T) {
	s := NewMemoryStore()
	id, _ := s.RegisterSong("Song")

	s.StoreHash(0xDEAD, id, 10)
	s.StoreHash(0xDEAD, id, 20)

	postings, err := s.Lookup(0xDEAD)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if len(postings) != 2 {
		t.Errorf("expected 2 postings, got %d", len(postings))
	}
}

func TestMemoryStore_LookupMiss(t *testing.T) {
	s := NewMemoryStore()
	postings, err := s.Lookup(0x12345678)
	if err != nil {
		t.Fatalf("unexpected error on miss: %v", err)
	}
	if len(postings) != 0 {
		t.Errorf("expected empty result for unknown hash, got %d postings", len(postings))
	}
}

func TestMemoryStore_StoreBatch(t *testing.T) {
	s := NewMemoryStore()
	id, _ := s.RegisterSong("Batch Song")

	hashes := []uint32{0xAA, 0xBB, 0xCC}
	offsets := []int{1, 2, 3}
	if err := s.StoreBatch(hashes, id, offsets); err != nil {
		t.Fatalf("StoreBatch: %v", err)
	}

	for i, h := range hashes {
		p, _ := s.Lookup(h)
		if len(p) != 1 || p[0].TimeOffset != offsets[i] {
			t.Errorf("hash 0x%X: expected offset %d, got %+v", h, offsets[i], p)
		}
	}
}

func TestMemoryStore_StoreBatch_LengthMismatch(t *testing.T) {
	s := NewMemoryStore()
	id, _ := s.RegisterSong("Song")
	err := s.StoreBatch([]uint32{1, 2}, id, []int{1})
	if err == nil {
		t.Error("expected error for mismatched lengths")
	}
}

func TestMemoryStore_Stats(t *testing.T) {
	s := NewMemoryStore()
	id1, _ := s.RegisterSong("A")
	id2, _ := s.RegisterSong("B")

	s.StoreHash(0x01, id1, 0)
	s.StoreHash(0x01, id2, 5) // same hash, different song
	s.StoreHash(0x02, id1, 0)

	stats := s.Stats()
	if stats.Songs != 2 {
		t.Errorf("expected 2 songs, got %d", stats.Songs)
	}
	if stats.HashEntries != 2 { // 0x01 and 0x02
		t.Errorf("expected 2 hash entries, got %d", stats.HashEntries)
	}
	if stats.TotalPostings != 3 {
		t.Errorf("expected 3 total postings, got %d", stats.TotalPostings)
	}
}

func TestMemoryStore_ConcurrentAccess(t *testing.T) {
	// Race detector will catch data races if the store isn't properly locked.
	s := NewMemoryStore()
	id, _ := s.RegisterSong("Concurrent Song")

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func(offset int) {
			defer wg.Done()
			s.StoreHash(uint32(offset%10), id, offset)
		}(i)
		go func(h uint32) {
			defer wg.Done()
			s.Lookup(h)
		}(uint32(i % 10))
	}
	wg.Wait()
}
