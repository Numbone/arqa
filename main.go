package main

import (
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"io/fs"
	"log"
	"net/http"
	"os"
	"time"
)

//go:embed web
var webFS embed.FS

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	dbPath := flag.String("db", "diary.db", "path to SQLite database file")
	seedPath := flag.String("seed", "trips.json", "JSON file to seed an empty database from")
	flag.Parse()

	store, err := NewStore(*dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer store.Close()

	if err := seedIfEmpty(store, *seedPath); err != nil {
		log.Fatalf("seed: %v", err)
	}

	log.Printf("driver shift diary listening on %s (db: %s)", *addr, *dbPath)
	if err := http.ListenAndServe(*addr, newHandler(store)); err != nil {
		log.Fatal(err)
	}
}

type server struct {
	store *Store
}

// newHandler wires the API routes and the embedded web client.
func newHandler(store *Store) http.Handler {
	srv := &server{store: store}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/trips", srv.handleTrips)
	mux.HandleFunc("POST /api/trips", srv.handleAddTrip)
	mux.HandleFunc("GET /api/summary", srv.handleSummary)

	static, _ := fs.Sub(webFS, "web")
	mux.Handle("/", http.FileServer(http.FS(static)))
	return mux
}

// handleTrips returns the trips for ?date=YYYY-MM-DD.
func (s *server) handleTrips(w http.ResponseWriter, r *http.Request) {
	date := r.URL.Query().Get("date")
	if !validDate(date) {
		writeError(w, http.StatusBadRequest, "date query parameter must be YYYY-MM-DD")
		return
	}
	trips, err := s.store.TripsByDate(date)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if trips == nil {
		trips = []Trip{}
	}
	writeJSON(w, http.StatusOK, trips)
}

// handleSummary returns the day summary for ?date=YYYY-MM-DD.
func (s *server) handleSummary(w http.ResponseWriter, r *http.Request) {
	date := r.URL.Query().Get("date")
	if !validDate(date) {
		writeError(w, http.StatusBadRequest, "date query parameter must be YYYY-MM-DD")
		return
	}
	trips, err := s.store.TripsByDate(date)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, ComputeSummary(date, trips))
}

// handleAddTrip adds a trip. It returns 201 for a new trip, 200 when the trip
// already existed (idempotent on ID / content fingerprint), 400 for invalid
// data and 409 when the ID is taken by a trip with different data.
func (s *server) handleAddTrip(w http.ResponseWriter, r *http.Request) {
	var t Trip
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&t); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	if err := t.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	stored, created, err := s.store.Add(t)
	if errors.Is(err, ErrIDConflict) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, stored)
}

// seedIfEmpty imports trips from a JSON file into an empty database. It is a
// no-op if the database already has trips or the seed file is absent.
func seedIfEmpty(store *Store, path string) error {
	n, err := store.Count()
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var trips []Trip
	if err := json.Unmarshal(data, &trips); err != nil {
		return err
	}
	for _, t := range trips {
		if _, _, err := store.Add(t); err != nil {
			return err
		}
	}
	log.Printf("seeded %d trips from %s", len(trips), path)
	return nil
}

func validDate(s string) bool {
	if s == "" {
		return false
	}
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
