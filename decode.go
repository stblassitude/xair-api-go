package xair

import "strings"

// decodePush converts a pushed message's first argument into the same
// units the equivalent getter on the node/mixin types returns, or nil if
// the address is not one this package knows how to convert.
//
// The parameter subtrees under every channel-like root are identical
// across mixer families and differ only in the root address and its
// padding width (WIRE_PROTOCOL.md §8), so the conversion is selected by
// matching the tail of the address rather than by enumerating every
// root: "/ch/01/mix/fader" and "/lr/mix/fader" both resolve through
// "mix/fader", "/bus/1/eq/3/g" and "/mtx/02/eq/3/g" both through
// "eq/#/g".
func decodePush(addr string, args []any) any {
	segs := strings.Split(strings.TrimPrefix(addr, "/"), "/")
	n := len(segs)

	// Longest tail first, so that a specific key always wins over a
	// shorter, more generic one — a hypothetical future "gate/filter/on"
	// entry would take precedence over the catch-all "on".
	for k := min(pushKeyMaxSegments, n); k >= 1; k-- {
		if d, ok := pushDecoders[pushKey(segs[n-k:])]; ok {
			return d(args)
		}
	}
	// Subtrees whose leaf segment is a variable token that is neither a
	// fixed name nor a plain number (a GEQ band like "31.5" or "1k", a
	// link pair like "1-2").
	if n >= 2 {
		if d, ok := pushSubtreeDecoders[segs[n-2]]; ok {
			return d(args)
		}
	}
	return nil
}

// pushKeyMaxSegments is the longest key in pushDecoders, in segments.
const pushKeyMaxSegments = 3

// pushKey joins address segments into a pushDecoders key, replacing each
// purely numeric segment — a 1-based index such as an EQ band, a send
// number, or a headamp number — with "#".
func pushKey(segs []string) string {
	parts := make([]string, len(segs))
	for i, s := range segs {
		if isIndex(s) {
			parts[i] = "#"
		} else {
			parts[i] = s
		}
	}
	return strings.Join(parts, "/")
}

func isIndex(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// pushDecoder converts a pushed message's arguments to a decoded value,
// or nil if they are not of the type the address's parameter uses.
type pushDecoder func(args []any) any

// pushDecoders maps an address tail (see pushKey) to the conversion the
// corresponding getter applies, per the parameter tables in
// WIRE_PROTOCOL.md §§6 and 8.
var pushDecoders = map[string]pushDecoder{
	// Every parameter named "on" anywhere in the address space is an
	// i-typed boolean: /mix/on, /gate/on, /dyn/on, /insert/on, /eq/on,
	// /gate/filter/on, /dyn/filter/on, /dca/N/on.
	"on": decodeBool,

	// Per-channel /config.
	"config/name":   decodeString,
	"config/color":  decodeInt,
	"config/insrc":  decodeInt,
	"config/rtnsrc": decodeInt,

	// /preamp.
	"preamp/rtntrim": decodeLin(-18, 18),
	"preamp/rtnsw":   decodeBool,
	"preamp/invert":  decodeBool,
	"preamp/hpon":    decodeBool,
	"preamp/hpf":     decodeLog(20, 400),

	// /gate.
	"gate/mode":        decodeEnum(gateModes),
	"gate/thr":         decodeLin(-80, 0),
	"gate/range":       decodeLin(3, 60),
	"gate/attack":      decodeLin(0, 120),
	"gate/hold":        decodeLog(0.02, 2000),
	"gate/release":     decodeLog(5, 4000),
	"gate/keysrc":      decodeInt,
	"gate/filter/type": decodeInt,
	"gate/filter/f":    decodeLog(20, 20000),

	// /dyn.
	"dyn/mode":        decodeEnum(dynModes),
	"dyn/det":         decodeEnum(dynDets),
	"dyn/env":         decodeEnum(dynEnvs),
	"dyn/thr":         decodeLin(-60, 0),
	"dyn/ratio":       decodeRatio,
	"dyn/knee":        decodeLin(0, 5),
	"dyn/mgain":       decodeLin(0, 24),
	"dyn/attack":      decodeLin(0, 120),
	"dyn/hold":        decodeLog(0.02, 2000),
	"dyn/release":     decodeLog(5, 4000),
	"dyn/mix":         decodeLin(0, 100),
	"dyn/keysrc":      decodeInt,
	"dyn/auto":        decodeBool,
	"dyn/filter/type": decodeInt,
	"dyn/filter/f":    decodeLog(20, 20000),

	// /insert.
	"insert/sel": decodeInt,

	// /eq and its bands.
	"eq/mode":   decodeEnum(eqModes),
	"eq/#/type": decodeInt,
	"eq/#/f":    decodeLog(20, 20000),
	"eq/#/g":    decodeLin(-15, 15),
	"eq/#/q":    decodeQuality,

	// /mix, including the per-bus/fx sends.
	"mix/fader":   decodeDB,
	"mix/lr":      decodeBool,
	"mix/#/level": decodeDB,

	// /grp and /automix.
	"grp/dca":        decodeInt,
	"grp/mute":       decodeInt,
	"automix/group":  decodeInt,
	"automix/weight": decodeLin(-12, 12),

	// Roots with no channel-like subtree of their own.
	"fx/#/type":         decodeInt,
	"headamp/#/gain":    decodeLin(-12, 60),
	"headamp/#/phantom": decodeBool,

	// Global /config.
	"config/amixenable": decodeBool,
	"config/amixlock":   decodeBool,
	"linkcfg/eq":        decodeBool,
	"linkcfg/dyn":       decodeBool,
	"linkcfg/fdrmute":   decodeBool,
	"mute/#":            decodeBool,
	"solo/level":        decodeDB,
	"solo/source":       decodeInt,
	"solo/sourcetrim":   decodeLin(-18, 18),
	"solo/chmode":       decodeBool,
	"solo/busmode":      decodeBool,
	"solo/dimatt":       decodeLin(-40, 0),
	"solo/dim":          decodeBool,
	"solo/mono":         decodeBool,
	"solo/mute":         decodeBool,
	"solo/dimfpl":       decodeBool,
}

// pushSubtreeDecoders maps the segment *above* the leaf, for the
// subtrees whose leaf segment is neither a fixed parameter name nor a
// plain index: /geq/<band> and /config/{chlink,buslink}/<i>-<i+1>.
var pushSubtreeDecoders = map[string]pushDecoder{
	"geq":     decodeLin(-15, 15),
	"chlink":  decodeBool,
	"buslink": decodeBool,
}

// pushFloat and pushInt reuse the reply-argument helpers; the address
// they take is only used to build an error message, which a decoder
// discards in favor of reporting "not decodable" as a nil value.
func pushFloat(args []any) (float64, bool) {
	v, err := argFloat32(args, "")
	return float64(v), err == nil
}

func pushInt(args []any) (int32, bool) {
	v, err := argInt32(args, "")
	return v, err == nil
}

func decodeBool(args []any) any {
	v, ok := pushInt(args)
	if !ok {
		return nil
	}
	return v == 1
}

func decodeInt(args []any) any {
	v, ok := pushInt(args)
	if !ok {
		return nil
	}
	return v
}

func decodeString(args []any) any {
	v, err := argString(args, "")
	if err != nil {
		return nil
	}
	return v
}

func decodeDB(args []any) any {
	v, ok := pushFloat(args)
	if !ok {
		return nil
	}
	return dBGet(v)
}

func decodeLin(min, max float64) pushDecoder {
	return func(args []any) any {
		v, ok := pushFloat(args)
		if !ok {
			return nil
		}
		return round1(linGet(min, max, v))
	}
}

func decodeLog(min, max float64) pushDecoder {
	return func(args []any) any {
		v, ok := pushFloat(args)
		if !ok {
			return nil
		}
		return round1(logGet(min, max, v))
	}
}

// decodeQuality mirrors EQBand.Quality's inverted wire encoding.
func decodeQuality(args []any) any {
	v, ok := pushFloat(args)
	if !ok {
		return nil
	}
	return round1(logGet(0.3, 10, 1.0-v))
}

// decodeEnum mirrors the getters that turn an i-typed index into one of
// a fixed, ordered set of option names (WIRE_PROTOCOL.md §6).
func decodeEnum(opts []string) pushDecoder {
	return func(args []any) any {
		i, ok := pushInt(args)
		if !ok {
			return nil
		}
		name, err := enumName(opts, i)
		if err != nil {
			return nil
		}
		return name
	}
}

// decodeRatio mirrors Dyn.Ratio: the wire carries an index, the decoded
// value is the ratio it names.
func decodeRatio(args []any) any {
	i, ok := pushInt(args)
	if !ok || i < 0 || int(i) >= len(dynRatios) {
		return nil
	}
	return dynRatios[i]
}
