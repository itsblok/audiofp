// audiofp — audio fingerprinting engine CLI.
//
// Usage:
//
//	audiofp serve  [--port 8080] [--db ./fp.db]
//	audiofp index  --db ./fp.db [--name "Song Name"] audio.wav
//	audiofp query  --db ./fp.db audio.wav
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/itsblok/audiofp/api"
	"github.com/itsblok/audiofp/storage"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	logger := log.New(os.Stdout, "[audiofp] ", log.LstdFlags)

	switch os.Args[1] {
	case "serve":
		cmdServe(os.Args[2:], logger)
	case "index":
		cmdIndex(os.Args[2:], logger)
	case "query":
		cmdQuery(os.Args[2:], logger)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %q\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

// cmdServe starts the HTTP API server and blocks until SIGINT/SIGTERM.
// On signal, it initiates graceful shutdown: drains in-flight requests
// for up to 10 seconds, then closes the SQLite store cleanly.
func cmdServe(args []string, logger *log.Logger) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	port := fs.String("port", "8080", "HTTP port to listen on")
	dbPath := fs.String("db", "fingerprints.db", "path to SQLite database")
	fs.Parse(args)

	store, err := storage.OpenSQLiteStore(*dbPath)
	if err != nil {
		logger.Fatalf("open store: %v", err)
	}
	defer store.Close()

	stats := store.Stats()
	logger.Printf("database: %s (%d songs, %d total postings)",
		*dbPath, stats.Songs, stats.TotalPostings)

	pipeline := api.NewPipeline()
	srv := api.NewServer(store, pipeline, logger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := srv.Serve(ctx, ":"+*port); err != nil {
		logger.Fatalf("server: %v", err)
	}
}

// cmdIndex fingerprints a WAV file and persists it to the database.
func cmdIndex(args []string, logger *log.Logger) {
	fs := flag.NewFlagSet("index", flag.ExitOnError)
	dbPath := fs.String("db", "fingerprints.db", "path to SQLite database")
	name := fs.String("name", "", "song name (default: filename)")
	fs.Parse(args)

	wavPath := fs.Arg(0)
	if wavPath == "" {
		fmt.Fprintln(os.Stderr, "usage: audiofp index [--db path] [--name name] audio.wav")
		os.Exit(1)
	}
	if *name == "" {
		base := filepath.Base(wavPath)
		*name = strings.TrimSuffix(base, filepath.Ext(base))
	}

	store, err := storage.OpenSQLiteStore(*dbPath)
	if err != nil {
		logger.Fatalf("open store: %v", err)
	}
	defer store.Close()

	f, err := os.Open(wavPath)
	if err != nil {
		logger.Fatalf("open: %v", err)
	}
	defer f.Close()

	start := time.Now()
	pipeline := api.NewPipeline()
	result, err := pipeline.Index(f, *name, store)
	if err != nil {
		logger.Fatalf("index: %v", err)
	}

	fmt.Printf("Indexed: %q  song_id=%d  peaks=%d  hashes=%d  (%s)\n",
		result.Name, result.SongID, result.Peaks, result.Hashes,
		time.Since(start).Round(time.Millisecond))
}

// cmdQuery identifies an unknown WAV clip against the database.
func cmdQuery(args []string, logger *log.Logger) {
	fs := flag.NewFlagSet("query", flag.ExitOnError)
	dbPath := fs.String("db", "fingerprints.db", "path to SQLite database")
	fs.Parse(args)

	wavPath := fs.Arg(0)
	if wavPath == "" {
		fmt.Fprintln(os.Stderr, "usage: audiofp query [--db path] audio.wav")
		os.Exit(1)
	}

	store, err := storage.OpenSQLiteStore(*dbPath)
	if err != nil {
		logger.Fatalf("open store: %v", err)
	}
	defer store.Close()

	f, err := os.Open(wavPath)
	if err != nil {
		logger.Fatalf("open: %v", err)
	}
	defer f.Close()

	start := time.Now()
	pipeline := api.NewPipeline()
	result, ok, err := pipeline.Query(f, store)
	elapsed := time.Since(start).Round(time.Millisecond)

	if err != nil {
		logger.Fatalf("query: %v", err)
	}
	if !ok {
		fmt.Printf("No match found (%s)\n", elapsed)
		os.Exit(2)
	}

	fmt.Printf("Match: %q  song_id=%d  confidence=%.1f%%  votes=%d  align=%d  (%s)\n",
		result.Name, result.SongID, result.Confidence*100,
		result.Votes, result.Alignment, elapsed)
}

func printUsage() {
	fmt.Fprintln(os.Stderr, `audiofp — audio fingerprinting engine

Usage:
  audiofp serve [--port 8080] [--db ./fp.db]
  audiofp index [--db ./fp.db] [--name "Song Name"] audio.wav
  audiofp query [--db ./fp.db] audio.wav

Commands:
  serve   Start the HTTP API server  (POST /songs, POST /query, GET /songs)
  index   Fingerprint and store a WAV file
  query   Identify an unknown WAV clip

Examples:
  audiofp serve --port 9000 --db ./library.db
  audiofp index --db ./library.db --name "A Major" chord.wav
  audiofp query --db ./library.db snippet.wav`)
}
