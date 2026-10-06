package main

import (
	"database/sql"
	"errors"
	"time"

	_ "modernc.org/sqlite"
)

// Store persists trips in a SQLite database.
type Store struct {
	db *sql.DB
}

const schema = `
CREATE TABLE IF NOT EXISTS trips (
	id         TEXT PRIMARY KEY,
	start      TEXT NOT NULL,
	end        TEXT NOT NULL,
	amount     INTEGER NOT NULL,
	payment    TEXT NOT NULL,
	commission INTEGER NOT NULL,
	fingerprint TEXT NOT NULL UNIQUE
);`

// ErrIDConflict is returned when a trip is submitted with the ID of an already
// stored trip but with different data.
var ErrIDConflict = errors.New("another trip with this id already exists")

// NewStore opens (and migrates) the SQLite database at path. A missing file is
// created. Call Close when done.
func NewStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	// SQLite allows a single writer; serialize access to avoid "database is
	// locked" errors under concurrent requests.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

// Close releases the database handle.
func (s *Store) Close() error { return s.db.Close() }

// Add validates and stores a trip, returning the stored trip. A trip is a
// duplicate when its ID or its content fingerprint is already stored; then
// created is false and the existing trip is returned unchanged (idempotent
// add). Reusing an ID for different data fails with ErrIDConflict.
func (s *Store) Add(t Trip) (stored Trip, created bool, err error) {
	if err := t.Validate(); err != nil {
		return Trip{}, false, err
	}
	fp := t.Fingerprint()
	if t.ID == "" {
		t.ID = fp
	}

	res, err := s.db.Exec(
		`INSERT OR IGNORE INTO trips (id, start, end, amount, payment, commission, fingerprint)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.Start.Format(time.RFC3339), t.End.Format(time.RFC3339),
		t.Amount, t.Payment, t.Commission, fp,
	)
	if err != nil {
		return Trip{}, false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return Trip{}, false, err
	}
	if n == 1 {
		return t, true, nil
	}

	// Not inserted: either the ID or the fingerprint is already taken.
	existing, err := s.getOne(`id = ?`, t.ID)
	switch {
	case err == nil:
		if !existing.SameContent(t) {
			return Trip{}, false, ErrIDConflict
		}
		return existing, false, nil
	case errors.Is(err, sql.ErrNoRows):
		existing, err = s.getOne(`fingerprint = ?`, fp)
		return existing, false, err
	default:
		return Trip{}, false, err
	}
}

// TripsByDate returns the trips for the given day (see Trip.Day), sorted by
// start time. The day is the date prefix of the RFC3339 start value, which
// preserves each trip's own UTC offset.
func (s *Store) TripsByDate(date string) ([]Trip, error) {
	rows, err := s.db.Query(
		`SELECT id, start, end, amount, payment, commission
		 FROM trips WHERE substr(start, 1, 10) = ? ORDER BY start`, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Trip
	for rows.Next() {
		t, err := scanTrip(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// Count returns the total number of stored trips.
func (s *Store) Count() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM trips`).Scan(&n)
	return n, err
}

func (s *Store) getOne(where string, arg any) (Trip, error) {
	row := s.db.QueryRow(
		`SELECT id, start, end, amount, payment, commission FROM trips WHERE `+where, arg)
	return scanTrip(row)
}

// scanner is satisfied by both *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

func scanTrip(sc scanner) (Trip, error) {
	var (
		t                Trip
		startStr, endStr string
	)
	if err := sc.Scan(&t.ID, &startStr, &endStr, &t.Amount, &t.Payment, &t.Commission); err != nil {
		return Trip{}, err
	}
	var err error
	if t.Start, err = time.Parse(time.RFC3339, startStr); err != nil {
		return Trip{}, err
	}
	if t.End, err = time.Parse(time.RFC3339, endStr); err != nil {
		return Trip{}, err
	}
	return t, nil
}
