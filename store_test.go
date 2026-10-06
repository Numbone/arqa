package main

import (
	"errors"
	"path/filepath"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := NewStore(filepath.Join(t.TempDir(), "diary.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// countOn is a test helper that returns the number of stored trips for a day.
func countOn(t *testing.T, s *Store, date string) int {
	t.Helper()
	trips, err := s.TripsByDate(date)
	if err != nil {
		t.Fatalf("TripsByDate: %v", err)
	}
	return len(trips)
}

// Re-posting a trip with the same explicit ID must not create a duplicate.
func TestAddDedupByID(t *testing.T) {
	s := newTestStore(t)
	trip := mustTrip(t, "t1", "2026-10-01T08:10:00+05:00", "2026-10-01T08:32:00+05:00", 2400, PaymentCard, 360)

	if _, created, err := s.Add(trip); err != nil || !created {
		t.Fatalf("first Add: created=%v err=%v, want created=true", created, err)
	}
	if _, created, err := s.Add(trip); err != nil || created {
		t.Fatalf("second Add: created=%v err=%v, want created=false", created, err)
	}

	if got := countOn(t, s, "2026-10-01"); got != 1 {
		t.Errorf("stored trips = %d, want 1", got)
	}
}

// Re-posting the same trip content without an ID must not create a duplicate,
// because the content fingerprint resolves to the same ID.
func TestAddDedupByFingerprint(t *testing.T) {
	s := newTestStore(t)
	trip := mustTrip(t, "", "2026-10-01T08:10:00+05:00", "2026-10-01T08:32:00+05:00", 2400, PaymentCard, 360)

	first, created, err := s.Add(trip)
	if err != nil || !created {
		t.Fatalf("first Add: created=%v err=%v, want created=true", created, err)
	}
	if first.ID == "" {
		t.Fatal("expected generated ID, got empty")
	}

	second, created, err := s.Add(trip)
	if err != nil || created {
		t.Fatalf("second Add: created=%v err=%v, want created=false", created, err)
	}
	if second.ID != first.ID {
		t.Errorf("fingerprint IDs differ: %q vs %q", second.ID, first.ID)
	}
	if got := countOn(t, s, "2026-10-01"); got != 1 {
		t.Errorf("stored trips = %d, want 1", got)
	}
}

func TestAddValidation(t *testing.T) {
	s := newTestStore(t)
	tests := map[string]Trip{
		"zero amount":      mustTrip(t, "a", "2026-10-01T08:00:00+05:00", "2026-10-01T08:30:00+05:00", 0, PaymentCard, 0),
		"end before start": mustTrip(t, "b", "2026-10-01T09:00:00+05:00", "2026-10-01T08:30:00+05:00", 1000, PaymentCard, 0),
		"bad payment":      mustTrip(t, "c", "2026-10-01T08:00:00+05:00", "2026-10-01T08:30:00+05:00", 1000, "crypto", 0),
		"neg commission":   mustTrip(t, "d", "2026-10-01T08:00:00+05:00", "2026-10-01T08:30:00+05:00", 1000, PaymentCard, -5),
	}
	for name, trip := range tests {
		t.Run(name, func(t *testing.T) {
			if _, _, err := s.Add(trip); err == nil {
				t.Error("expected validation error, got nil")
			}
		})
	}
}

// A store reloaded from disk must still deduplicate against persisted trips.
func TestAddPersistsAndDedupsAfterReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "diary.db")
	s1, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	trip := mustTrip(t, "t1", "2026-10-01T08:10:00+05:00", "2026-10-01T08:32:00+05:00", 2400, PaymentCard, 360)
	if _, _, err := s1.Add(trip); err != nil {
		t.Fatalf("Add: %v", err)
	}
	s1.Close()

	s2, err := NewStore(path)
	if err != nil {
		t.Fatalf("reload NewStore: %v", err)
	}
	defer s2.Close()
	if _, created, err := s2.Add(trip); err != nil || created {
		t.Fatalf("Add after reload: created=%v err=%v, want created=false", created, err)
	}
	if got := countOn(t, s2, "2026-10-01"); got != 1 {
		t.Errorf("stored trips after reload = %d, want 1", got)
	}
}

// The same instant written with a different UTC offset is the same ride.
func TestAddDedupAcrossOffsets(t *testing.T) {
	s := newTestStore(t)
	local := mustTrip(t, "", "2026-10-01T08:10:00+05:00", "2026-10-01T08:32:00+05:00", 2400, PaymentCard, 360)
	utc := mustTrip(t, "", "2026-10-01T03:10:00Z", "2026-10-01T03:32:00Z", 2400, PaymentCard, 360)

	if _, created, err := s.Add(local); err != nil || !created {
		t.Fatalf("first Add: created=%v err=%v, want created=true", created, err)
	}
	if _, created, err := s.Add(utc); err != nil || created {
		t.Fatalf("Add in UTC: created=%v err=%v, want created=false", created, err)
	}
}

// A trip stored with an explicit ID (e.g. from the seed file) and re-posted
// without an ID or under another ID must resolve to the stored one.
func TestAddDedupSeededTripWithoutID(t *testing.T) {
	s := newTestStore(t)
	seeded := mustTrip(t, "t1", "2026-10-01T08:10:00+05:00", "2026-10-01T08:32:00+05:00", 2400, PaymentCard, 360)
	if _, _, err := s.Add(seeded); err != nil {
		t.Fatalf("seed Add: %v", err)
	}

	for _, id := range []string{"", "t99"} {
		repost := seeded
		repost.ID = id
		got, created, err := s.Add(repost)
		if err != nil || created {
			t.Fatalf("repost id=%q: created=%v err=%v, want created=false", id, created, err)
		}
		if got.ID != "t1" {
			t.Errorf("repost id=%q returned ID %q, want t1", id, got.ID)
		}
	}
	if got := countOn(t, s, "2026-10-01"); got != 1 {
		t.Errorf("stored trips = %d, want 1", got)
	}
}

// Reusing an ID for a different ride must be rejected, not silently ignored.
func TestAddIDConflict(t *testing.T) {
	s := newTestStore(t)
	first := mustTrip(t, "t1", "2026-10-01T08:10:00+05:00", "2026-10-01T08:32:00+05:00", 2400, PaymentCard, 360)
	other := mustTrip(t, "t1", "2026-10-01T09:05:00+05:00", "2026-10-01T09:20:00+05:00", 1500, PaymentCash, 225)

	if _, _, err := s.Add(first); err != nil {
		t.Fatalf("first Add: %v", err)
	}
	if _, _, err := s.Add(other); !errors.Is(err, ErrIDConflict) {
		t.Fatalf("Add with taken ID: err=%v, want ErrIDConflict", err)
	}
	if got := countOn(t, s, "2026-10-01"); got != 1 {
		t.Errorf("stored trips = %d, want 1", got)
	}
}
