package main

import (
	"testing"
	"time"
)

func mustTrip(t *testing.T, id, start, end string, amount int64, payment string, commission int64) Trip {
	t.Helper()
	s, err := time.Parse(time.RFC3339, start)
	if err != nil {
		t.Fatalf("parse start: %v", err)
	}
	e, err := time.Parse(time.RFC3339, end)
	if err != nil {
		t.Fatalf("parse end: %v", err)
	}
	return Trip{ID: id, Start: s, End: e, Amount: amount, Payment: payment, Commission: commission}
}

func TestComputeSummary(t *testing.T) {
	trips := []Trip{
		mustTrip(t, "t1", "2026-10-01T08:10:00+05:00", "2026-10-01T08:32:00+05:00", 2400, PaymentCard, 360),
		mustTrip(t, "t2", "2026-10-01T09:05:00+05:00", "2026-10-01T09:20:00+05:00", 1500, PaymentCash, 225),
		mustTrip(t, "t3", "2026-10-01T10:00:00+05:00", "2026-10-01T10:30:00+05:00", 3000, PaymentCard, 450),
	}

	got := ComputeSummary("2026-10-01", trips)

	if got.Trips != 3 {
		t.Errorf("Trips = %d, want 3", got.Trips)
	}
	if got.Revenue != 6900 {
		t.Errorf("Revenue = %d, want 6900", got.Revenue)
	}
	if got.Commission != 1035 {
		t.Errorf("Commission = %d, want 1035", got.Commission)
	}
	if got.Net != 5865 {
		t.Errorf("Net = %d, want 5865", got.Net)
	}
	if got.Cash != (Breakdown{Trips: 1, Amount: 1500}) {
		t.Errorf("Cash = %+v, want {1 1500}", got.Cash)
	}
	if got.Card != (Breakdown{Trips: 2, Amount: 5400}) {
		t.Errorf("Card = %+v, want {2 5400}", got.Card)
	}
}

func TestComputeSummaryEmpty(t *testing.T) {
	got := ComputeSummary("2026-10-02", nil)

	want := Summary{Date: "2026-10-02"}
	if got != want {
		t.Errorf("empty summary = %+v, want %+v", got, want)
	}
}
