package xair

import (
	"math"
	"testing"
)

func almostEqual(a, b, eps float64) bool { return math.Abs(a-b) <= eps }

func TestLinRoundTrip(t *testing.T) {
	wire := linSet(-18, 18, 5.5)
	got := linGet(-18, 18, wire)
	if !almostEqual(got, 5.5, 1e-9) {
		t.Fatalf("lin round trip: got %v, want 5.5", got)
	}
}

func TestLogRoundTrip(t *testing.T) {
	wire := logSet(20, 20000, 1000)
	got := logGet(20, 20000, wire)
	if !almostEqual(got, 1000, 1e-6) {
		t.Fatalf("log round trip: got %v, want 1000", got)
	}
}

func TestDBTaperBreakpoints(t *testing.T) {
	cases := []struct {
		wire float64
		db   float64
	}{
		{1.0, 10},
		{0.75, 0},
		{0.5, -10},
		{0.25, -30},
		{0.0625, -60},
		{0.0, -90},
	}
	for _, c := range cases {
		if got := dBGet(c.wire); got != c.db {
			t.Errorf("dBGet(%v) = %v, want %v", c.wire, got, c.db)
		}
	}
}

func TestDBTaperRoundTrip(t *testing.T) {
	for _, db := range []float64{10, 0, -10, -30, -60, -90} {
		wire := dBSet(db)
		got := dBGet(wire)
		if !almostEqual(got, db, 0.05) {
			t.Errorf("dB round trip for %v: got %v", db, got)
		}
	}
}
