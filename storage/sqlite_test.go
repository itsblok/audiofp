package storage

import (
	"path/filepath"
	"testing"
)

// openTestDB opens an in-memory SQLite store for isolated unit tests.
func openTestDB(t *testing.T) *SQLiteStore {
	t.Helper()
	s, err := OpenSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("OpenSQLiteStore(:memory:): %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestSQLite_RegisterAndSongName(t *testing.T) {
	s := openTestDB(t)
	id, err := s.RegisterSong("Pink Floyd - Comfortably Numb")
	if err != nil {
		t.Fatalf("RegisterSong: %v", err)
	}
	if id == 0 {
		t.Error("expected non-zero song ID")
	}
	name, ok := s.SongName(id)
	if !ok || name != "Pink Floyd - Comfortably Numb" {
		t.Errorf("SongName(%d) = %q, %v", id, name, ok)
	}
}

func TestSQLite_DuplicateSong(t *testing.T) {
	s := openTestDB(t)
	s.RegisterSong("Duplicate")
	_, err := s.RegisterSong("Duplicate")
	if err != ErrDuplicateSong {
		t.Errorf("expected ErrDuplicateSong, got %v", err)
	}
}

func TestSQLite_SongNameNotFound(t *testing.T) {
	s := openTestDB(t)
	_, ok := s.SongName(9999)
	if ok {
		t.Error("expected ok=false for unknown ID")
	}
}

func TestSQLite_StoreHashAndLookup(t *testing.T) {
	s := openTestDB(t)
	id, _ := s.RegisterSong("Song A")

	s.StoreHash(0xABCD1234, id, 42)
	s.StoreHash(0xABCD1234, id, 100)

	postings, err := s.Lookup(0xABCD1234)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if len(postings) != 2 {
		t.Errorf("expected 2 postings, got %d", len(postings))
	}
}

func TestSQLite_LookupMiss(t *testing.T) {
	s := openTestDB(t)
	postings, err := s.Lookup(0xDEADBEEF)
	if err != nil {
		t.Fatalf("unexpected Lookup error: %v", err)
	}
	if len(postings) != 0 {
		t.Errorf("expected empty result, got %d postings", len(postings))
	}
}

func TestSQLite_StoreBatch(t *testing.T) {
	s := openTestDB(t)
	id, _ := s.RegisterSong("Batch Song")

	hashes := []uint32{0x01, 0x02, 0x03, 0x04, 0x05}
	offsets := []int{10, 20, 30, 40, 50}
	if err := s.StoreBatch(hashes, id, offsets); err != nil {
		t.Fatalf("StoreBatch: %v", err)
	}

	for i, h := range hashes {
		postings, _ := s.Lookup(h)
		if len(postings) != 1 || postings[0].TimeOffset != offsets[i] {
			t.Errorf("hash 0x%X: expected offset %d, got %+v", h, offsets[i], postings)
		}
	}
}

func TestSQLite_StoreBatch_LengthMismatch(t *testing.T) {
	s := openTestDB(t)
	id, _ := s.RegisterSong("Song")
	err := s.StoreBatch([]uint32{1, 2}, id, []int{1})
	if err == nil {
		t.Error("expected error for length mismatch")
	}
}

func TestSQLite_Stats(t *testing.T) {
	s := openTestDB(t)
	id1, _ := s.RegisterSong("A")
	id2, _ := s.RegisterSong("B")

	s.StoreHash(0x01, id1, 0)
	s.StoreHash(0x01, id2, 5) // same hash, different song
	s.StoreHash(0x02, id1, 0)

	stats := s.Stats()
	if stats.Songs != 2 {
		t.Errorf("songs: expected 2, got %d", stats.Songs)
	}
	if stats.HashEntries != 2 {
		t.Errorf("hashEntries: expected 2 (distinct), got %d", stats.HashEntries)
	}
	if stats.TotalPostings != 3 {
		t.Errorf("totalPostings: expected 3, got %d", stats.TotalPostings)
	}
}

func TestSQLite_Persistence(t *testing.T) {
	// Write to a real file, close, reopen, and verify data survived.
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "fp.db")

	// --- Session 1: write ---
	{
		s, err := OpenSQLiteStore(dbPath)
		if err != nil {
			t.Fatalf("open session 1: %v", err)
		}
		id, _ := s.RegisterSong("Persisted Song")
		s.StoreHash(0xCAFEBABE, id, 77)
		s.Close()
	}

	// --- Session 2: read ---
	{
		s, err := OpenSQLiteStore(dbPath)
		if err != nil {
			t.Fatalf("open session 2: %v", err)
		}
		defer s.Close()

		postings, err := s.Lookup(0xCAFEBABE)
		if err != nil {
			t.Fatalf("Lookup after reopen: %v", err)
		}
		if len(postings) != 1 {
			t.Fatalf("expected 1 posting after reopen, got %d", len(postings))
		}
		if postings[0].TimeOffset != 77 {
			t.Errorf("TimeOffset: expected 77, got %d", postings[0].TimeOffset)
		}

		name, ok := s.SongName(postings[0].SongID)
		if !ok || name != "Persisted Song" {
			t.Errorf("SongName after reopen: %q, %v", name, ok)
		}
	}
}

func TestSQLite_HashBoundaryValues(t *testing.T) {
	// Verify uint32 max value (0xFFFFFFFF) round-trips correctly.
	// This is a potential issue because SQLite INTEGER is signed int64,
	// and a naive cast could interpret 0xFFFFFFFF as -1.
	s := openTestDB(t)
	id, _ := s.RegisterSong("Boundary")

	const maxUint32 = uint32(0xFFFFFFFF)
	s.StoreHash(maxUint32, id, 0)

	postings, err := s.Lookup(maxUint32)
	if err != nil {
		t.Fatalf("Lookup(maxUint32): %v", err)
	}
	if len(postings) != 1 {
		t.Errorf("expected 1 posting for max uint32 hash, got %d", len(postings))
	}
}

// TestSQLite_ImplementsStore verifies SQLiteStore satisfies the Store interface
// at compile time. No runtime logic needed — this is a type-assertion check.
func TestSQLite_ImplementsStore(t *testing.T) {
	var _ Store = (*SQLiteStore)(nil)
}
