package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"
)

// Payment methods.
const (
	PaymentCash = "cash"
	PaymentCard = "card"
)

// Trip is a single driver ride. Money values are integer amounts (tenge).
type Trip struct {
	ID         string    `json:"id"`
	Start      time.Time `json:"start"`
	End        time.Time `json:"end"`
	Amount     int64     `json:"amount"`
	Payment    string    `json:"payment"`
	Commission int64     `json:"commission"`
}

// Day returns the calendar date of the trip's start, in the start timestamp's
// own offset (e.g. "2026-10-01"). Trips are grouped into days by this value.
func (t Trip) Day() string {
	return t.Start.Format("2006-01-02")
}

// Fingerprint is a deterministic identity derived from the trip's content.
// The store keeps it unique, so the same ride is never stored twice, with or
// without an explicit ID. Times are normalized to UTC so that one instant
// written with different offsets ("+05:00" vs "Z") yields the same value.
func (t Trip) Fingerprint() string {
	seed := strings.Join([]string{
		t.Start.UTC().Format(time.RFC3339),
		t.End.UTC().Format(time.RFC3339),
		strconv.FormatInt(t.Amount, 10),
		t.Payment,
	}, "|")
	sum := sha256.Sum256([]byte(seed))
	return "t_" + hex.EncodeToString(sum[:8])
}

// SameContent reports whether two trips describe the same ride with the same
// data (ID is ignored, times are compared as instants).
func (t Trip) SameContent(o Trip) bool {
	return t.Start.Equal(o.Start) && t.End.Equal(o.End) &&
		t.Amount == o.Amount && t.Payment == o.Payment && t.Commission == o.Commission
}

// Validate checks the business rules for a trip.
func (t Trip) Validate() error {
	if t.Amount <= 0 {
		return errors.New("amount must be greater than 0")
	}
	if t.Commission < 0 {
		return errors.New("commission must not be negative")
	}
	if t.Start.IsZero() || t.End.IsZero() {
		return errors.New("start and end are required")
	}
	if !t.End.After(t.Start) {
		return errors.New("end must be after start")
	}
	if t.Payment != PaymentCash && t.Payment != PaymentCard {
		return errors.New(`payment must be "cash" or "card"`)
	}
	return nil
}

// Breakdown is a per-payment-method aggregate.
type Breakdown struct {
	Trips  int   `json:"trips"`
	Amount int64 `json:"amount"`
}

// Summary is the aggregate for a single day.
type Summary struct {
	Date       string    `json:"date"`
	Trips      int       `json:"trips"`
	Revenue    int64     `json:"revenue"`
	Commission int64     `json:"commission"`
	Net        int64     `json:"net"` // revenue - commission ("на руки")
	Cash       Breakdown `json:"cash"`
	Card       Breakdown `json:"card"`
}

// ComputeSummary aggregates the given trips into a day summary. Callers pass
// only the trips belonging to date; this function does no filtering.
func ComputeSummary(date string, trips []Trip) Summary {
	s := Summary{Date: date, Trips: len(trips)}
	for _, t := range trips {
		s.Revenue += t.Amount
		s.Commission += t.Commission
		switch t.Payment {
		case PaymentCash:
			s.Cash.Trips++
			s.Cash.Amount += t.Amount
		case PaymentCard:
			s.Card.Trips++
			s.Card.Amount += t.Amount
		}
	}
	s.Net = s.Revenue - s.Commission
	return s
}
