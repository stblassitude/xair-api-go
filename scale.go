package xair

import "math"

// linGet/linSet and logGet/logSet implement the two generic 0.0-1.0
// wire-value curves used throughout the protocol (WIRE_PROTOCOL.md §6),
// mirroring xair-api-python's util.lin_get/lin_set/log_get/log_set.
func linGet(min, max, wire float64) float64 { return min + (max-min)*wire }
func linSet(min, max, val float64) float64  { return (val - min) / (max - min) }

func logGet(min, max, wire float64) float64 { return min * math.Exp(math.Log(max/min)*wire) }
func logSet(min, max, val float64) float64  { return math.Log(val/min) / math.Log(max/min) }

// dBGet converts a 0.0-1.0 wire fader/level value to dB using the mixer's
// piecewise taper (WIRE_PROTOCOL.md §6), mirroring util.db_from.
func dBGet(wire float64) float64 {
	switch {
	case wire >= 1:
		return 10
	case wire >= 0.5:
		return round1(40*wire - 30)
	case wire >= 0.25:
		return round1(80*wire - 50)
	case wire >= 0.0625:
		return round1(160*wire - 70)
	case wire >= 0:
		return round1(480*wire - 90)
	default:
		return -90
	}
}

// dBSet is the inverse of dBGet, mirroring util.db_to.
func dBSet(db float64) float64 {
	switch {
	case db >= 10:
		return 1
	case db >= -10:
		return (db + 30) / 40
	case db >= -30:
		return (db + 50) / 80
	case db >= -60:
		return (db + 70) / 160
	case db >= -90:
		return (db + 90) / 480
	default:
		return 0
	}
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }
