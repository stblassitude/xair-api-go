package xair

import (
	"fmt"
	"sync"
	"time"
)

// MeterSource is a family of level meters the mixer reports, named
// independently of which /meters bank a given mixer family happens to
// carry it in (WIRE_PROTOCOL.md §1a).
type MeterSource int

const (
	// MeterStrip is an input strip's own level, ahead of its fader.
	MeterStrip MeterSource = iota + 1

	// MeterBus is a mix bus master's level.
	MeterBus

	// MeterFXSend is an effect send's level.
	MeterFXSend

	// MeterFXReturn is an effect return's level.
	MeterFXReturn

	// MeterAuxReturn is an aux return's level.
	MeterAuxReturn

	// MeterDCA is a DCA group's level, after the strips it controls.
	MeterDCA

	// MeterMain is the main output's level, after its fader.
	MeterMain

	// MeterMatrix is a matrix bus master's level (X32 only).
	MeterMatrix

	// MeterMono is the mono/centre output's level (X32 only).
	MeterMono

	// MeterMonitor is a monitor/phones output's level.
	MeterMonitor
)

func (s MeterSource) String() string {
	switch s {
	case MeterStrip:
		return "strip"
	case MeterBus:
		return "bus"
	case MeterFXSend:
		return "fx send"
	case MeterFXReturn:
		return "fx return"
	case MeterAuxReturn:
		return "aux return"
	case MeterDCA:
		return "dca"
	case MeterMain:
		return "main"
	case MeterMatrix:
		return "matrix"
	case MeterMono:
		return "mono"
	case MeterMonitor:
		return "monitor"
	default:
		return fmt.Sprintf("meter source %d", int(s))
	}
}

// Meter selects a single level meter. Number is 1-based and counts
// individual meter values, not stereo pairs: a stereo source occupies
// two consecutive numbers, left then right, so the X-Air aux return's
// two sides are AuxReturnMeter(1) and AuxReturnMeter(2).
type Meter struct {
	Source MeterSource
	Number int
}

func (m Meter) String() string { return fmt.Sprintf("%s %d", m.Source, m.Number) }

// StripMeter selects input strip n's level.
func StripMeter(n int) Meter { return Meter{MeterStrip, n} }

// BusMeter selects mix bus n's master level.
func BusMeter(n int) Meter { return Meter{MeterBus, n} }

// FXSendMeter selects effect send n's level. X-Air sends are mono, one
// per FX slot; X32 sends are stereo, so n counts sides — FXSendMeter(1)
// and FXSendMeter(2) are FX slot 1's left and right.
func FXSendMeter(n int) Meter { return Meter{MeterFXSend, n} }

// FXReturnMeter selects effect return n's level. Returns are stereo on
// both families, so n counts sides: FXReturnMeter(1) and
// FXReturnMeter(2) are FX slot 1's left and right.
func FXReturnMeter(n int) Meter { return Meter{MeterFXReturn, n} }

// AuxReturnMeter selects aux return n's level. The X-Air aux return is
// stereo, so n is 1 or 2 for its left and right; the X32's eight aux
// inputs are mono, so n is 1 to 8.
func AuxReturnMeter(n int) Meter { return Meter{MeterAuxReturn, n} }

// DCAMeter selects DCA group n's level.
func DCAMeter(n int) Meter { return Meter{MeterDCA, n} }

// MainMeter selects the main output's level: 1 for left, 2 for right.
func MainMeter(n int) Meter { return Meter{MeterMain, n} }

// MatrixMeter selects matrix bus n's master level (X32 only).
func MatrixMeter(n int) Meter { return Meter{MeterMatrix, n} }

// MonoMeter selects the mono/centre output's level (X32 only).
func MonoMeter() Meter { return Meter{MeterMono, 1} }

// MonitorMeter selects the monitor output's level: 1 for left, 2 for
// right.
func MonitorMeter(n int) Meter { return Meter{MeterMonitor, n} }

// meterRange places one MeterSource inside one meter bank: count values
// starting at offset, in the order the bank lists them
// (WIRE_PROTOCOL.md §1a).
//
// perGroup and groupStride cover the one bank whose values for a source
// are not contiguous — X32 /meters/9, which interleaves each FX slot's
// two sends with its two returns — by taking perGroup values every
// groupStride. Zero means contiguous.
type meterRange struct {
	bank        int
	args        []int32
	offset      int
	count       int
	perGroup    int
	groupStride int
}

func (r meterRange) index(n int) int {
	i := n - 1
	if r.perGroup > 0 && r.groupStride > 0 {
		return r.offset + (i/r.perGroup)*r.groupStride + i%r.perGroup
	}
	return r.offset + i
}

// xairMeterLayout maps the X-Air/MR meter banks. Bank 1 holds every
// input, bus, send and output level in one block, which is why a
// typical level display needs only one subscription; the DCAs are the
// exception, in bank 8 (WIRE_PROTOCOL.md §1a).
var xairMeterLayout = map[MeterSource]meterRange{
	MeterStrip:     {bank: 1, offset: 0, count: 16},
	MeterAuxReturn: {bank: 1, offset: 16, count: 2},
	MeterFXReturn:  {bank: 1, offset: 18, count: 8},
	MeterBus:       {bank: 1, offset: 26, count: 6},
	MeterFXSend:    {bank: 1, offset: 32, count: 4},
	MeterMain:      {bank: 1, offset: 36, count: 2},
	MeterMonitor:   {bank: 1, offset: 38, count: 2},
	MeterDCA:       {bank: 8, offset: 0, count: 4},
}

// x32MeterLayout maps the X32/M32 meter banks. Bank 0 holds the input
// and bus levels, but not the main outputs, which are in bank 2; the
// DCAs are in bank 5's group half, selected by its two arguments
// (WIRE_PROTOCOL.md §1a).
var x32MeterLayout = map[MeterSource]meterRange{
	MeterStrip:     {bank: 0, offset: 0, count: 32},
	MeterAuxReturn: {bank: 0, offset: 32, count: 8},
	MeterFXReturn:  {bank: 0, offset: 40, count: 8},
	MeterBus:       {bank: 0, offset: 48, count: 16},
	MeterMatrix:    {bank: 0, offset: 64, count: 6},
	MeterMain:      {bank: 2, offset: 22, count: 2},
	MeterMono:      {bank: 2, offset: 24, count: 1},
	MeterDCA:       {bank: 5, args: []int32{0, 0}, offset: 16, count: 8},
	MeterFXSend:    {bank: 9, offset: 0, count: 16, perGroup: 2, groupStride: 4},
}

func meterLayout(kindID KindID) map[MeterSource]meterRange {
	if kindID == KindX32 {
		return x32MeterLayout
	}
	return xairMeterLayout
}

// meterPoint is a resolved Meter: the bank to subscribe to and the index
// into that bank's values.
type meterPoint struct {
	bank  int
	args  []int32
	index int
}

// resolveMeter maps a Meter onto the given family's banks. The bounds it
// checks are the width of the meter bank, not the mixer model's channel
// count: a number a particular model does not physically have is inside
// the block all the same, and simply reads silence.
func resolveMeter(kindID KindID, m Meter) (meterPoint, error) {
	r, ok := meterLayout(kindID)[m.Source]
	if !ok {
		return meterPoint{}, fmt.Errorf("xair: %s has no %s meter", kindID, m.Source)
	}
	if m.Number < 1 || m.Number > r.count {
		return meterPoint{}, fmt.Errorf("xair: %s %s meter number %d is out of range 1 to %d",
			kindID, m.Source, m.Number, r.count)
	}
	return meterPoint{bank: r.bank, args: r.args, index: r.index(m.Number)}, nil
}

// MeterFrame is one reading of the meters passed to SubscribeLevels.
type MeterFrame struct {
	// Levels holds each meter's level in dBFS, floored at
	// MeterSilenceDB, in the same order as the meters given to
	// SubscribeLevels.
	Levels []float64
}

type meterOptions struct {
	interval time.Duration
}

// MeterOption configures SubscribeLevels.
type MeterOption func(*meterOptions)

// WithMeterInterval sets the update interval asked of the mixer. It is
// rounded up to a whole multiple of MeterBaseInterval and clamped to 99
// of them. Defaults to DefaultMeterInterval.
func WithMeterInterval(d time.Duration) MeterOption {
	return func(o *meterOptions) { o.interval = d }
}

// meterDelivery is one bank's block, tagged with the feed it came from.
type meterDelivery struct {
	feed int
	u    *MeterUpdate
}

// SubscribeLevels subscribes to whichever meter banks the given meters
// live in and delivers their levels, in the order asked for, as they
// arrive. The returned function cancels the subscription and closes the
// channel; it is idempotent.
//
// This resolves each Meter against this mixer's family, so callers
// choose meters by what they are ("input strip 3", "main left") rather
// than by bank and index; Client.SubscribeMeters is the level below,
// for a bank this package has no layout for.
//
// Most selections resolve to a single bank. Where they do not — X32
// input strips and main outputs are in different banks — every bank
// needed is subscribed to, and one frame is emitted per update of the
// bank holding the *first* meter in the list, carrying the most recent
// value received for each of the others. A meter whose bank has not
// reported yet reads MeterSilenceDB.
//
// The channel holds a single frame: if the consumer is not ready, that
// frame is dropped in favor of the next, since a stale level is worth
// nothing. Renewal is automatic for as long as the subscription is
// live (see Client.SubscribeMeters).
func (m *Mixer) SubscribeLevels(meters []Meter, opts ...MeterOption) (<-chan *MeterFrame, func(), error) {
	if len(meters) == 0 {
		return nil, nil, fmt.Errorf("xair: SubscribeLevels needs at least one meter")
	}

	o := meterOptions{interval: DefaultMeterInterval}
	for _, opt := range opts {
		opt(&o)
	}

	// Resolve every meter first, so that an unsupported one fails
	// before anything has been subscribed to.
	points := make([]meterPoint, len(meters))
	for i, meter := range meters {
		p, err := resolveMeter(m.KindID, meter)
		if err != nil {
			return nil, nil, err
		}
		points[i] = p
	}

	// Group the meters by the bank subscription that feeds them,
	// keeping first-appearance order so that feed 0 — the one frames
	// are clocked off — is the bank holding the first meter asked for.
	feedOf := make([]int, len(points))
	var requests []MeterRequest
	byKey := make(map[string]int, len(points))
	for i, p := range points {
		req := MeterRequest{Bank: p.bank, Args: p.args, Interval: o.interval}
		key := req.key()
		feed, ok := byKey[key]
		if !ok {
			feed = len(requests)
			byKey[key] = feed
			requests = append(requests, req)
		}
		feedOf[i] = feed
	}

	cancels := make([]func(), 0, len(requests))
	channels := make([]<-chan *MeterUpdate, 0, len(requests))
	for _, req := range requests {
		ch, cancel, err := m.Client.SubscribeMeters(req)
		if err != nil {
			for _, c := range cancels {
				c()
			}
			return nil, nil, err
		}
		cancels = append(cancels, cancel)
		channels = append(channels, ch)
	}

	// Fan the per-bank channels into one stream so the levels can be
	// assembled by a single goroutine without locking.
	merged := make(chan meterDelivery)
	var wg sync.WaitGroup
	for feed, ch := range channels {
		wg.Add(1)
		go func(feed int, ch <-chan *MeterUpdate) {
			defer wg.Done()
			for u := range ch {
				merged <- meterDelivery{feed: feed, u: u}
			}
		}(feed, ch)
	}
	go func() {
		wg.Wait()
		close(merged)
	}()

	frames := make(chan *MeterFrame, 1)
	go func() {
		defer close(frames)

		levels := make([]float64, len(points))
		for i := range levels {
			levels[i] = MeterSilenceDB
		}
		for d := range merged {
			for i, p := range points {
				if feedOf[i] != d.feed || p.index >= len(d.u.Values) {
					continue
				}
				levels[i] = d.u.Values[p.index]
			}
			if d.feed != 0 {
				continue
			}
			frame := &MeterFrame{Levels: append([]float64(nil), levels...)}
			select {
			case frames <- frame:
			default:
			}
		}
	}()

	var once sync.Once
	return frames, func() {
		once.Do(func() {
			for _, cancel := range cancels {
				cancel()
			}
		})
	}, nil
}
