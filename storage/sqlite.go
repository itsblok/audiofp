// Package storage — SQLite-backed persistent fingerprint store.
//
// SQLiteStore implements the Store interface using a local SQLite database.
// It is the production-grade counterpart to MemoryStore: survives restarts,
// supports arbitrarily large song libraries, and benefits from SQLite's
// mature query planner for hash lookups.
//
// Schema design rationale:
//
//	songs(id, name): simple identity table. AUTOINCREMENT gives us stable
//	numeric IDs that are cheaply stored as 4-byte integers in fingerprints.
//
//	fingerprints(hash, song_id, time_offset): the inverted index.
//	hash is the lookup key — we query by hash constantly, never by song_id
//	or time_offset alone. The index on hash turns Lookup() from a full
//	table scan into a B-tree seek: O(log N + K) where K = result count.
//
//	There is intentionally no PRIMARY KEY on fingerprints. Duplicate hashes
//	from the same song at different offsets are valid and expected.
package storage

import (
	"database/sql"
	"fmt"
	"sync"

	_ "github.com/mattn/go-sqlite3" // registers the "sqlite3" driver
)

// schema is applied once when the database is first opened.
// CREATE IF NOT EXISTS makes it safe to run on an existing database.
const schema = `
CREATE TABLE IF NOT EXISTS songs (
    id   INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT    UNIQUE NOT NULL
);

CREATE TABLE IF NOT EXISTS fingerprints (
    hash        INTEGER NOT NULL,
    song_id     INTEGER NOT NULL,
    time_offset INTEGER NOT NULL
);

-- This index is the performance-critical piece.
-- Without it, every Lookup() is a full fingerprints table scan.
CREATE INDEX IF NOT EXISTS idx_fp_hash ON fingerprints(hash);

-- Optimize SQLite for write-heavy workloads (indexing phase).
PRAGMA journal_mode = WAL;
PRAGMA synchronous  = NORMAL;
PRAGMA cache_size   = -64000; -- 64 MB page cache
`

// SQLiteStore is a persistent Store backed by a SQLite database file.
// All methods are safe for concurrent use via a single db connection pool.
type SQLiteStore struct {
	db *sql.DB
	mu sync.Mutex // serializes write operations (WAL handles concurrent reads)
}

// OpenSQLiteStore opens (or creates) a SQLite database at the given file path.
// Pass ":memory:" for an in-process test database that doesn't touch disk.
func OpenSQLiteStore(path string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, fmt.Errorf("sqlite: open %q: %w", path, err)
	}

	// Single writer connection is simplest and avoids SQLITE_BUSY under WAL.
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite: apply schema: %w", err)
	}

	return &SQLiteStore{db: db}, nil
}

// Close releases the database connection. Call this when done with the store.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

func (s *SQLiteStore) RegisterSong(name string) (uint32, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Use INSERT OR IGNORE + SELECT to handle the duplicate case without
	// relying on error code inspection (more portable).
	res, err := s.db.Exec(`INSERT OR IGNORE INTO songs(name) VALUES(?)`, name)
	if err != nil {
		return 0, fmt.Errorf("sqlite: RegisterSong: %w", err)
	}

	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		// Name already existed — return ErrDuplicateSong to match MemoryStore.
		return 0, ErrDuplicateSong
	}

	lastID, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("sqlite: RegisterSong last id: %w", err)
	}
	return uint32(lastID), nil
}

func (s *SQLiteStore) SongName(id uint32) (string, bool) {
	var name string
	err := s.db.QueryRow(`SELECT name FROM songs WHERE id = ?`, id).Scan(&name)
	if err == sql.ErrNoRows {
		return "", false
	}
	if err != nil {
		return "", false
	}
	return name, true
}

func (s *SQLiteStore) StoreHash(hash uint32, songID uint32, timeOffset int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(
		`INSERT INTO fingerprints(hash, song_id, time_offset) VALUES(?,?,?)`,
		int64(hash), songID, timeOffset,
	)
	return err
}

// StoreBatch inserts all hashes for a song in a single transaction.
// This is dramatically faster than N individual StoreHash calls:
// SQLite flushes to WAL on every commit; batching 500 inserts into one
// transaction reduces that to a single flush.
func (s *SQLiteStore) StoreBatch(hashes []uint32, songID uint32, offsets []int) error {
	if len(hashes) != len(offsets) {
		return fmt.Errorf("storage: hashes length %d != offsets length %d",
			len(hashes), len(offsets))
	}
	if len(hashes) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("sqlite: StoreBatch begin tx: %w", err)
	}

	stmt, err := tx.Prepare(
		`INSERT INTO fingerprints(hash, song_id, time_offset) VALUES(?,?,?)`,
	)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("sqlite: StoreBatch prepare: %w", err)
	}
	defer stmt.Close()

	for i, h := range hashes {
		if _, err := stmt.Exec(int64(h), songID, offsets[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("sqlite: StoreBatch insert[%d]: %w", i, err)
		}
	}

	return tx.Commit()
}

// Lookup returns all postings for the given hash.
// The idx_fp_hash index makes this a B-tree seek rather than a full scan.
// uint32 hashes are stored as int64 to fit SQLite's INTEGER type safely.
func (s *SQLiteStore) Lookup(hash uint32) ([]Posting, error) {
	rows, err := s.db.Query(
		`SELECT song_id, time_offset FROM fingerprints WHERE hash = ?`,
		int64(hash),
	)
	if err != nil {
		return nil, fmt.Errorf("sqlite: Lookup: %w", err)
	}
	defer rows.Close()

	var postings []Posting
	for rows.Next() {
		var p Posting
		if err := rows.Scan(&p.SongID, &p.TimeOffset); err != nil {
			return nil, fmt.Errorf("sqlite: Lookup scan: %w", err)
		}
		postings = append(postings, p)
	}
	return postings, rows.Err()
}

func (s *SQLiteStore) Stats() StoreStats {
	var stats StoreStats

	s.db.QueryRow(`SELECT COUNT(*) FROM songs`).Scan(&stats.Songs)

	// COUNT(DISTINCT hash) is correct but O(N) on large tables.
	// Acceptable for a research system; in production, maintain a counter.
	s.db.QueryRow(`SELECT COUNT(DISTINCT hash) FROM fingerprints`).Scan(&stats.HashEntries)
	s.db.QueryRow(`SELECT COUNT(*) FROM fingerprints`).Scan(&stats.TotalPostings)

	return stats
}
