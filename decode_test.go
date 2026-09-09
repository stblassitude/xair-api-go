package xair

import "testing"

// TestDecodePush locks in that a pushed value comes out in the same
// units the equivalent getter returns, across every conversion in
// WIRE_PROTOCOL.md §6 and across both address families (§8).
func TestDecodePush(t *testing.T) {
	cases := []struct {
		addr string
		arg  any
		want any
	}{
		// Fader/level dB taper, on both an X-Air and an X32 root.
		{"/ch/01/mix/fader", float32(0.5), -10.0},
		{"/lr/mix/fader", float32(1), 10.0},
		{"/main/st/mix/fader", float32(0), -90.0},
		{"/ch/01/mix/03/level", float32(0.75), 0.0},
		{"/rtn/aux/mix/10/level", float32(0.25), -30.0},
		{"/config/solo/level", float32(0.5), -10.0},

		// Booleans, including the address-space-wide "on".
		{"/ch/01/mix/on", int32(1), true},
		{"/dca/3/on", int32(0), false},
		{"/ch/01/gate/filter/on", int32(1), true},
		{"/config/mute/2", int32(1), true},
		{"/config/solo/mute", int32(0), false},
		{"/config/amixenable", int32(1), true},
		{"/config/linkcfg/eq", int32(1), true},
		{"/config/chlink/1-2", int32(1), true},
		{"/config/buslink/3-4", int32(0), false},
		{"/headamp/07/phantom", int32(1), true},
		{"/headamp/127/phantom", int32(0), false},

		// Linear and logarithmic curves.
		{"/ch/01/gate/thr", float32(0.5), -40.0},
		{"/ch/01/dyn/thr", float32(0.5), -30.0},
		{"/bus/1/dyn/mix", float32(0.25), 25.0},
		{"/ch/01/preamp/hpf", float32(0), 20.0},
		{"/ch/01/dyn/filter/f", float32(1), 20000.0},
		{"/ch/01/automix/weight", float32(1), 12.0},
		{"/headamp/01/gain", float32(0), -12.0},
		{"/mtx/02/eq/3/g", float32(0.5), 0.0},
		{"/bus/1/geq/31.5", float32(1), 15.0},
		{"/lr/geq/1k25", float32(0), -15.0},

		// The inverted EQ quality encoding.
		{"/ch/01/eq/1/q", float32(1), 0.3},

		// Enumerated choices and the ratio lookup table decode to what
		// their getters return, not to the wire index.
		{"/ch/01/gate/mode", int32(3), "gate"},
		{"/ch/01/dyn/det", int32(1), "rms"},
		{"/bus/1/eq/mode", int32(0), "peq"},
		{"/ch/01/dyn/ratio", int32(11), 100.0},

		// Plain integers and strings.
		{"/ch/01/config/color", int32(4), int32(4)},
		{"/dca/1/config/name", "Vocals", "Vocals"},
		{"/ch/01/grp/mute", int32(6), int32(6)},
		{"/fx/2/type", int32(5), int32(5)},
		{"/ch/01/eq/2/type", int32(3), int32(3)},
		{"/ch/01/insert/sel", int32(9), int32(9)},

		// Not decodable: an address outside §8's space, a value of the
		// wrong type, an out-of-range enum index, and a meter block,
		// which never reaches this path at all — it is routed to its own
		// subscribers (§1a) before the push decoders see it.
		{"/xinfo", "10.0.0.1", nil},
		{"/some/unknown/param", float32(0.5), nil},
		{"/ch/01/mix/fader", "loud", nil},
		{"/ch/01/gate/mode", int32(9), nil},
		{"/meters/1", int32(0), nil},
	}

	for _, c := range cases {
		got := decodePush(c.addr, []any{c.arg})
		if got != c.want {
			t.Errorf("decodePush(%q, [%v]) = %v (%T), want %v (%T)", c.addr, c.arg, got, got, c.want, c.want)
		}
	}
}

// TestDecodePushWithoutArguments checks the GET-shaped message (no
// arguments at all, WIRE_PROTOCOL.md §3) does not panic if one is ever
// echoed back at us.
func TestDecodePushWithoutArguments(t *testing.T) {
	if got := decodePush("/ch/01/mix/fader", nil); got != nil {
		t.Errorf("decodePush with no arguments = %v, want nil", got)
	}
}
