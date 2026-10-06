package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(newHandler(newTestStore(t)))
	t.Cleanup(srv.Close)
	return srv
}

func postTrip(t *testing.T, srv *httptest.Server, body string) *http.Response {
	t.Helper()
	res, err := http.Post(srv.URL+"/api/trips", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /api/trips: %v", err)
	}
	t.Cleanup(func() { res.Body.Close() })
	return res
}

func getJSON(t *testing.T, srv *httptest.Server, path string, v any) {
	t.Helper()
	res, err := http.Get(srv.URL + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: status %d, want 200", path, res.StatusCode)
	}
	if err := json.NewDecoder(res.Body).Decode(v); err != nil {
		t.Fatalf("GET %s: decode: %v", path, err)
	}
}

const (
	tripCard = `{"start":"2026-10-01T08:10:00+05:00","end":"2026-10-01T08:32:00+05:00","amount":2400,"payment":"card","commission":360}`
	tripCash = `{"start":"2026-10-01T09:05:00+05:00","end":"2026-10-01T09:20:00+05:00","amount":1500,"payment":"cash","commission":225}`
)

// Repeating the same POST must answer 200 and leave a single trip, and the
// day summary must reflect exactly the stored trips.
func TestAPIAddIsIdempotentAndSummarized(t *testing.T) {
	srv := newTestServer(t)

	for i, want := range []int{http.StatusCreated, http.StatusOK} {
		if got := postTrip(t, srv, tripCard).StatusCode; got != want {
			t.Fatalf("POST #%d: status %d, want %d", i+1, got, want)
		}
	}
	if got := postTrip(t, srv, tripCash).StatusCode; got != http.StatusCreated {
		t.Fatalf("POST cash: status %d, want 201", got)
	}

	var trips []Trip
	getJSON(t, srv, "/api/trips?date=2026-10-01", &trips)
	if len(trips) != 2 {
		t.Fatalf("trips = %d, want 2", len(trips))
	}

	var sum Summary
	getJSON(t, srv, "/api/summary?date=2026-10-01", &sum)
	want := Summary{
		Date: "2026-10-01", Trips: 2, Revenue: 3900, Commission: 585, Net: 3315,
		Cash: Breakdown{Trips: 1, Amount: 1500}, Card: Breakdown{Trips: 1, Amount: 2400},
	}
	if sum != want {
		t.Errorf("summary = %+v, want %+v", sum, want)
	}
}

func TestAPIAddErrors(t *testing.T) {
	srv := newTestServer(t)
	if got := postTrip(t, srv, `{"id":"t1",`+tripCard[1:]).StatusCode; got != http.StatusCreated {
		t.Fatalf("seed POST: status %d, want 201", got)
	}

	tests := map[string]struct {
		body string
		want int
	}{
		"zero amount":      {`{"start":"2026-10-01T08:00:00+05:00","end":"2026-10-01T08:30:00+05:00","amount":0,"payment":"cash","commission":0}`, http.StatusBadRequest},
		"end before start": {`{"start":"2026-10-01T09:00:00+05:00","end":"2026-10-01T08:30:00+05:00","amount":1000,"payment":"cash","commission":0}`, http.StatusBadRequest},
		"time without tz":  {`{"start":"2026-10-01T08:00:00","end":"2026-10-01T08:30:00","amount":1000,"payment":"cash","commission":0}`, http.StatusBadRequest},
		"unknown field":    {`{"start":"2026-10-01T08:00:00+05:00","end":"2026-10-01T08:30:00+05:00","amount":1000,"payment":"cash","commission":0,"tip":5}`, http.StatusBadRequest},
		"taken id":         {`{"id":"t1",` + tripCash[1:], http.StatusConflict},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			res := postTrip(t, srv, tc.body)
			if res.StatusCode != tc.want {
				t.Fatalf("status %d, want %d", res.StatusCode, tc.want)
			}
			var body map[string]string
			if err := json.NewDecoder(res.Body).Decode(&body); err != nil || body["error"] == "" {
				t.Errorf("want JSON {\"error\": ...}, got err=%v body=%v", err, body)
			}
		})
	}
}

func TestAPIRejectsBadDate(t *testing.T) {
	srv := newTestServer(t)
	for _, path := range []string{"/api/trips", "/api/summary?date=01.10.2026"} {
		res, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusBadRequest {
			t.Errorf("GET %s: status %d, want 400", path, res.StatusCode)
		}
	}
}
