package xair

import (
	"encoding/binary"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	goosc "github.com/hypebeast/go-osc/osc"
)

// MetersAddress is the OSC address a client sends, naming a meter bank,
// to subscribe its own UDP source address/port to that bank's periodic
// binary meter blocks (WIRE_PROTOCOL.md §1a).
const MetersAddress = "/meters"

// meterAddressPrefix is the prefix of the address a meter block is
// pushed to; the bank number follows it.
const meterAddressPrefix = MetersAddress + "/"

// MeterBaseInterval is the mixer's own meter update cycle: a
// subscription's time factor is a whole multiple of it, so 50 ms (20 Hz)
// is both the fastest achievable rate and the granularity of every
// slower one (WIRE_PROTOCOL.md §1a).
const MeterBaseInterval = 50 * time.Millisecond

// DefaultMeterInterval is the update interval requested when a
// MeterRequest does not name one. 100 ms (10 Hz) is fast enough for a
// level display to look continuous while asking the mixer for a fifth of
// the datagrams the 50 ms floor would.
const DefaultMeterInterval = 100 * time.Millisecond

// DefaultMeterRenewInterval is how often a Client with at least one
// active meter subscription re-sends the /meters request that keeps it
// alive. Like /xremote, a meter subscription lapses if it is not renewed
// within ~10s (WIRE_PROTOCOL.md §1a); this is half that, because a lapse
// here does not merely delay one update but freezes a running level
// display for as long as it takes to notice.
const DefaultMeterRenewInterval = 5 * time.Second

// MeterSilenceDB is the level reported for a fully silent meter, and the
// floor every decoded meter value is clamped to. It is the value of the
// most negative int16 an X-Air blob can carry (-32768/256), which the
// firmware uses for "no signal" (WIRE_PROTOCOL.md §1a).
const MeterSilenceDB = -128.0

// meterBuffer is how many meter blocks a subscriber may fall behind by
// before further ones are dropped. Meter blocks are a continuous stream
// in which only the newest reading has any value, so this is
// deliberately small: a consumer that cannot keep up wants the current
// level next, not a backlog of stale ones.
const meterBuffer = 8

// MeterEncoding is how a mixer family encodes the values inside a meter
// block. The two families differ (WIRE_PROTOCOL.md §1a), so this is set
// from the KindID by Connect, or with WithClientMeterEncoding on a
// bare Client.
type MeterEncoding int

const (
	// MeterEncodingInt16DB is the X-Air/MR encoding: signed 16-bit
	// little-endian values in units of 1/256 dB.
	MeterEncodingInt16DB MeterEncoding = iota

	// MeterEncodingFloat32Linear is the X32 encoding: 32-bit
	// little-endian floats holding a linear audio level, 1.0 being
	// digital full scale.
	MeterEncodingFloat32Linear
)

// MeterRequest names one meter bank to subscribe to.
type MeterRequest struct {
	// Bank is the N in /meters/N. Which banks exist, and what the
	// values in each one are, differs by mixer family
	// (WIRE_PROTOCOL.md §1a) — Mixer.SubscribeLevels resolves that
	// mapping for you.
	Bank int

	// Args are the bank's own selector arguments, for the banks that
	// take some (X-Air /meters/0 takes the channel to meter; X32
	// /meters/5 takes a channel-block and a group-block id). Leave nil
	// for banks that take none.
	Args []int32

	// Interval is the update interval to ask the mixer for. It is
	// rounded up to a whole multiple of MeterBaseInterval, so the
	// stream is never faster than requested, and clamped to the
	// protocol's 99-multiple ceiling (just under 5s). Zero means
	// DefaultMeterInterval.
	Interval time.Duration
}

func (r MeterRequest) address() string {
	return meterAddressPrefix + strconv.Itoa(r.Bank)
}

// key identifies the wire subscription this request needs, so that
// callers asking for the same bank with the same arguments share one.
// The interval is deliberately not part of it: the mixer streams a bank
// to a subscribed socket at one rate, so sharing is by what is metered,
// not by how often (see the tf handling in SubscribeMeters).
func (r MeterRequest) key() string {
	parts := make([]string, 0, len(r.Args)+1)
	parts = append(parts, strconv.Itoa(r.Bank))
	for _, a := range r.Args {
		parts = append(parts, strconv.Itoa(int(a)))
	}
	return strings.Join(parts, "/")
}

// meterTimeFactor converts a requested interval to the protocol's time
// factor: the number of MeterBaseInterval cycles between two meter
// blocks, an integer in 1..99 (WIRE_PROTOCOL.md §1a). It rounds up, so
// the resulting stream is never faster than the caller asked for.
func meterTimeFactor(d time.Duration) int32 {
	if d <= 0 {
		d = DefaultMeterInterval
	}
	tf := (int64(d) + int64(MeterBaseInterval) - 1) / int64(MeterBaseInterval)
	if tf < 1 {
		tf = 1
	}
	if tf > 99 {
		tf = 99
	}
	return int32(tf)
}

// MeterUpdate is one meter block pushed by the mixer to a subscribed
// client. Updates are shared by all subscribers to the same bank and
// must be treated as read-only.
type MeterUpdate struct {
	// Address is the OSC address the block arrived on, e.g.
	// "/meters/1"; Bank is the number in it.
	Address string
	Bank    int

	// Values holds the block's levels in dBFS, floored at
	// MeterSilenceDB, converted from whichever encoding the mixer
	// family uses. Which meter each index is depends on the bank
	// (WIRE_PROTOCOL.md §1a).
	Values []float64

	// Blob is the raw OSC blob argument, count prefix included, for a
	// caller decoding a bank this package has no layout for.
	Blob []byte
}

// meterSubscription is one registered consumer of a meter bank. tf is
// the time factor this consumer asked for, kept so that the group can
// fall back to a slower rate once a faster consumer leaves.
type meterSubscription struct {
	group  *meterGroup
	ch     chan *MeterUpdate
	tf     int32
	closed bool
}

// meterGroup is one wire-level meter subscription — one /meters request
// being renewed — shared by every subscriber that asked for the same
// bank and arguments. tf is the fastest rate any of them asked for.
type meterGroup struct {
	key  string
	addr string
	args []int32
	tf   int32
	subs []*meterSubscription
	stop chan struct{}
}

// fastestTF is the smallest time factor among the group's subscribers,
// i.e. the rate the wire subscription has to run at to satisfy all of
// them. The caller must hold meterMu.
func (g *meterGroup) fastestTF() int32 {
	tf := int32(99)
	for _, s := range g.subs {
		if s.tf < tf {
			tf = s.tf
		}
	}
	return tf
}

// SubscribeMeters registers for one of the mixer's binary meter banks
// and returns the channel its blocks are delivered on plus a function
// that cancels the subscription.
//
// This is the protocol-level entry point: it subscribes to a bank as a
// whole and hands back every value in it, leaving the caller to know
// which index is which meter. Mixer.SubscribeLevels sits on top of it
// and picks out individual, named meters instead.
//
// The /meters request is sent immediately and re-sent every
// DefaultMeterRenewInterval (see WithClientMeterRenewInterval) for as
// long as any subscriber to that bank remains; the caller does not have
// to renew anything. Cancelling the last subscription to a bank stops
// the renewal and lets the mixer-side subscription lapse on its own —
// meter blocks keep arriving for up to ~10s after that, and are dropped.
//
// Any number of subscribers may be registered at once, and those asking
// for the same bank and arguments share a single wire subscription: the
// mixer streams a bank to a subscribed socket at one rate, so that
// shared subscription runs at the fastest interval any of them asked
// for, and one that asked for a slower rate simply receives the faster
// stream. The returned channel is buffered; once it is full, further
// blocks for that subscriber are dropped rather than stalling the
// receive loop, since a stale meter reading is worth nothing. The
// cancel function is idempotent.
func (c *Client) SubscribeMeters(req MeterRequest) (<-chan *MeterUpdate, func(), error) {
	if req.Bank < 0 {
		return nil, nil, fmt.Errorf("xair: meter bank %d is not a valid bank number", req.Bank)
	}

	s := &meterSubscription{
		ch: make(chan *MeterUpdate, meterBuffer),
		tf: meterTimeFactor(req.Interval),
	}

	c.meterMu.Lock()
	// Checked under meterMu, which Close takes only after closing done,
	// so a SubscribeMeters racing a Close either registers in time to
	// have its channel closed by Close or takes this branch.
	select {
	case <-c.done:
		c.meterMu.Unlock()
		close(s.ch)
		s.closed = true
		return s.ch, func() {}, nil
	default:
	}

	key := req.key()
	g := c.meterGroups[key]
	if g == nil {
		g = &meterGroup{
			key:  key,
			addr: req.address(),
			args: req.Args,
			tf:   s.tf,
			stop: make(chan struct{}),
		}
		if c.meterGroups == nil {
			c.meterGroups = make(map[string]*meterGroup)
		}
		c.meterGroups[key] = g
		go c.renewMeters(g)
	}
	s.group = g
	g.subs = append(g.subs, s)

	// A request is needed now if this is the bank's first subscriber, or
	// if this one wants it faster than the rate it is running at.
	resend := len(g.subs) == 1 || s.tf < g.tf
	if s.tf < g.tf {
		g.tf = s.tf
	}
	tf := g.tf
	c.meterMu.Unlock()

	// Sent outside the lock, but before returning, so that by the time
	// the caller has the channel the mixer has been asked to meter.
	if resend {
		c.sendMeters(g, tf)
	}

	return s.ch, func() { c.unsubscribeMeters(s) }, nil
}

func (c *Client) unsubscribeMeters(s *meterSubscription) {
	c.meterMu.Lock()
	defer c.meterMu.Unlock()

	// Already gone, either through an earlier call or through Close, so
	// the group it belonged to has been torn down too.
	if s.closed {
		return
	}

	g := s.group
	if g != nil {
		for i, existing := range g.subs {
			if existing == s {
				g.subs = append(g.subs[:i], g.subs[i+1:]...)
				break
			}
		}
	}
	close(s.ch)
	s.closed = true

	if g == nil {
		return
	}
	if len(g.subs) == 0 {
		delete(c.meterGroups, g.key)
		close(g.stop)
		return
	}
	// The subscriber that was driving the rate may be the one that just
	// left; the next renewal picks the slower rate up on its own.
	g.tf = g.fastestTF()
}

func (c *Client) closeMeterSubscriptions() {
	c.meterMu.Lock()
	defer c.meterMu.Unlock()

	for key, g := range c.meterGroups {
		for _, s := range g.subs {
			if !s.closed {
				close(s.ch)
				s.closed = true
			}
		}
		g.subs = nil
		close(g.stop)
		delete(c.meterGroups, key)
	}
}

// renewMeters re-sends a bank's /meters request on a ticker until the
// last subscriber to it goes away or the Client is closed. The mixer
// forgets a meter subscription that is not renewed within ~10s
// (WIRE_PROTOCOL.md §1a).
func (c *Client) renewMeters(g *meterGroup) {
	t := time.NewTicker(c.meterRenewInterval)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			c.meterMu.Lock()
			tf := g.tf
			c.meterMu.Unlock()
			c.sendMeters(g, tf)
		case <-g.stop:
			return
		case <-c.done:
			return
		}
	}
}

// sendMeters sends the bank's /meters request: the bank address, the
// bank's own arguments if it takes any, and the time factor last
// (WIRE_PROTOCOL.md §1a).
func (c *Client) sendMeters(g *meterGroup, tf int32) {
	args := make([]any, 0, len(g.args)+2)
	args = append(args, g.addr)
	for _, a := range g.args {
		args = append(args, a)
	}
	args = append(args, tf)
	if err := c.Send(MetersAddress, args...); err != nil {
		c.logger.Warn("xair: could not send /meters subscription renewal", "bank", g.addr, "err", err)
	}
}

// dispatchMeter offers an incoming message to the meter subscribers of
// the bank it is addressed to, and reports whether it was a meter block
// at all. Anything in the /meters/N address space is consumed here even
// with no subscriber left — which is what arrives for the ~10s a
// cancelled subscription takes to lapse — rather than falling through to
// the parameter-change subscribers, to whom a binary blob means nothing.
func (c *Client) dispatchMeter(msg *goosc.Message) bool {
	if !strings.HasPrefix(msg.Address, meterAddressPrefix) {
		return false
	}
	bank, err := strconv.Atoi(strings.TrimPrefix(msg.Address, meterAddressPrefix))
	if err != nil {
		return false
	}

	c.meterMu.Lock()
	var targets []*meterSubscription
	for _, g := range c.meterGroups {
		if g.addr == msg.Address {
			targets = append(targets, g.subs...)
		}
	}
	enc := c.meterEncoding
	c.meterMu.Unlock()

	if len(targets) == 0 {
		return true
	}

	blob, ok := meterBlob(msg)
	if !ok {
		c.logger.Warn("xair: meter block has no blob argument", "addr", msg.Address, "args", msg.Arguments)
		return true
	}
	values, err := decodeMeterBlob(blob, enc)
	if err != nil {
		c.logger.Warn("xair: could not decode meter block", "addr", msg.Address, "err", err)
		return true
	}
	u := &MeterUpdate{Address: msg.Address, Bank: bank, Values: values, Blob: blob}

	// Delivered under meterMu (never blocking, since the channel is
	// buffered and the send is non-blocking) so that a subscriber cannot
	// be closed out from under an in-flight delivery.
	c.meterMu.Lock()
	defer c.meterMu.Unlock()
	for _, s := range targets {
		if s.closed {
			continue
		}
		select {
		case s.ch <- u:
		default:
			c.logger.Debug("xair: dropping meter block, subscriber is not keeping up", "addr", msg.Address)
		}
	}
	return true
}

func meterBlob(msg *goosc.Message) ([]byte, bool) {
	if len(msg.Arguments) == 0 {
		return nil, false
	}
	b, ok := msg.Arguments[0].([]byte)
	return b, ok
}

// decodeMeterBlob converts a meter block's blob to levels in dBFS. The
// blob is a little-endian 32-bit count of values followed by that many
// values in the family's encoding (WIRE_PROTOCOL.md §1a); the
// big-endian byte length that precedes it on the wire is the OSC blob
// header, which the OSC layer has already consumed.
func decodeMeterBlob(blob []byte, enc MeterEncoding) ([]float64, error) {
	if len(blob) < 4 {
		return nil, fmt.Errorf("xair: meter blob is %d bytes, too short to hold a value count", len(blob))
	}
	n := int(int32(binary.LittleEndian.Uint32(blob[:4])))
	if n < 0 {
		return nil, fmt.Errorf("xair: meter blob declares %d values", n)
	}
	body := blob[4:]

	width := 2
	if enc == MeterEncodingFloat32Linear {
		width = 4
	}
	if n*width > len(body) {
		return nil, fmt.Errorf("xair: meter blob declares %d values but carries only %d bytes of data", n, len(body))
	}

	values := make([]float64, n)
	for i := range values {
		if enc == MeterEncodingFloat32Linear {
			lin := math.Float32frombits(binary.LittleEndian.Uint32(body[i*4:]))
			values[i] = linearToDB(float64(lin))
			continue
		}
		values[i] = float64(int16(binary.LittleEndian.Uint16(body[i*2:]))) / 256
	}
	return values, nil
}

// linearToDB converts an X32 meter's linear level to the dBFS scale the
// X-Air blocks already use, so that MeterUpdate.Values means the same
// thing on both families.
func linearToDB(lin float64) float64 {
	if lin <= 0 {
		return MeterSilenceDB
	}
	db := 20 * math.Log10(lin)
	if db < MeterSilenceDB {
		return MeterSilenceDB
	}
	return db
}
