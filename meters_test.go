package xair

import (
	"encoding/binary"
	"math"
	"testing"
	"time"
)

// testMeterRenewInterval keeps the renewal tests fast; the shipping
// default is DefaultMeterRenewInterval, asserted separately below.
const testMeterRenewInterval = 30 * time.Millisecond

// newTestMixer builds a Mixer around a fake mixer's port without
// Connect's /xinfo handshake, applying the same per-kind meter encoding
// Connect would.
func newTestMixer(t *testing.T, kindID KindID, port int, opts ...ClientOption) *Mixer {
	t.Helper()
	k := kindTable[kindID]
	all := append([]ClientOption{WithClientMeterEncoding(k.meterEncoding())}, opts...)
	c, err := NewClient("127.0.0.1", port, all...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return &Mixer{Client: c, KindID: kindID}
}

// int16MeterBlob builds an X-Air meter block payload: a little-endian
// count followed by that many little-endian 1/256 dB shorts
// (WIRE_PROTOCOL.md §1a).
func int16MeterBlob(db ...float64) []byte {
	blob := make([]byte, 4+2*len(db))
	binary.LittleEndian.PutUint32(blob, uint32(len(db)))
	for i, v := range db {
		binary.LittleEndian.PutUint16(blob[4+2*i:], uint16(int16(math.Round(v*256))))
	}
	return blob
}

// float32MeterBlob builds an X32 meter block payload: a little-endian
// count followed by that many little-endian linear floats.
func float32MeterBlob(lin ...float32) []byte {
	blob := make([]byte, 4+4*len(lin))
	binary.LittleEndian.PutUint32(blob, uint32(len(lin)))
	for i, v := range lin {
		binary.LittleEndian.PutUint32(blob[4+4*i:], math.Float32bits(v))
	}
	return blob
}

func recvMeterUpdate(t *testing.T, ch <-chan *MeterUpdate, timeout time.Duration) *MeterUpdate {
	t.Helper()
	select {
	case u, ok := <-ch:
		if !ok {
			t.Fatal("meter channel was closed while waiting for a block")
		}
		return u
	case <-time.After(timeout):
		t.Fatal("timed out waiting for a meter block")
		return nil
	}
}

func recvFrame(t *testing.T, ch <-chan *MeterFrame, timeout time.Duration) *MeterFrame {
	t.Helper()
	select {
	case f, ok := <-ch:
		if !ok {
			t.Fatal("level channel was closed while waiting for a frame")
		}
		return f
	case <-time.After(timeout):
		t.Fatal("timed out waiting for a level frame")
		return nil
	}
}

func TestMeterTimeFactor(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want int32
	}{
		// The rate this feature exists for: 10 Hz is two of the mixer's
		// own 50 ms cycles.
		{100 * time.Millisecond, 2},
		{0, 2}, // DefaultMeterInterval
		{50 * time.Millisecond, 1},
		{time.Millisecond, 1},
		{-time.Second, 2}, // treated as unset
		// Rounded up, so the stream is never faster than requested.
		{120 * time.Millisecond, 3},
		{99 * time.Millisecond, 2},
		{time.Second, 20},
		// Clamped to the protocol's 99-cycle ceiling.
		{time.Minute, 99},
	}
	for _, c := range cases {
		if got := meterTimeFactor(c.in); got != c.want {
			t.Errorf("meterTimeFactor(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestDecodeMeterBlob(t *testing.T) {
	// X-Air: signed shorts in 1/256 dB, the most negative of which is
	// exactly MeterSilenceDB.
	values, err := decodeMeterBlob(int16MeterBlob(0, -6, -60.5, MeterSilenceDB), MeterEncodingInt16DB)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []float64{0, -6, -60.5, MeterSilenceDB}
	if len(values) != len(want) {
		t.Fatalf("got %d values, want %d", len(values), len(want))
	}
	for i, w := range want {
		if math.Abs(values[i]-w) > 1e-9 {
			t.Errorf("value %d = %v, want %v dB", i, values[i], w)
		}
	}

	// X32: linear floats, converted to the same dB scale so that
	// MeterUpdate.Values means one thing on both families.
	values, err = decodeMeterBlob(float32MeterBlob(1, 0.5, 0), MeterEncodingFloat32Linear)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want = []float64{0, -6.0206, MeterSilenceDB}
	if len(values) != len(want) {
		t.Fatalf("got %d values, want %d", len(values), len(want))
	}
	for i, w := range want {
		if math.Abs(values[i]-w) > 1e-3 {
			t.Errorf("value %d = %v, want %v dB", i, values[i], w)
		}
	}

	// A truncated or nonsensical block is reported, not read past.
	if _, err := decodeMeterBlob([]byte{1, 2}, MeterEncodingInt16DB); err == nil {
		t.Error("expected an error for a blob too short to hold a count")
	}
	short := int16MeterBlob(0, 0, 0)[:6]
	if _, err := decodeMeterBlob(short, MeterEncodingInt16DB); err == nil {
		t.Error("expected an error for a blob declaring more values than it carries")
	}
}

// TestSubscribeMetersReceivesBlock covers the round trip: the client
// asks for a bank at a given rate, the mixer pushes a block to that
// bank's address, and the subscriber gets it decoded into dB.
func TestSubscribeMetersReceivesBlock(t *testing.T) {
	m := startPushMixer(t, nil)

	c, err := NewClient("127.0.0.1", m.port())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	first, cancelFirst, err := c.SubscribeMeters(MeterRequest{Bank: 1, Interval: 100 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer cancelFirst()
	second, cancelSecond, err := c.SubscribeMeters(MeterRequest{Bank: 1, Interval: 100 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer cancelSecond()

	// The bank address first, then the time factor: 10 Hz is two 50 ms
	// cycles (WIRE_PROTOCOL.md §1a).
	req := m.waitMeters(t, time.Second)
	if len(req.args) != 2 || req.args[0] != "/meters/1" || req.args[1] != int32(2) {
		t.Fatalf("/meters request args = %v, want [/meters/1 2]", req.args)
	}

	// Both subscribers asked for the same bank at the same rate, so
	// they share one wire subscription and both see every block.
	select {
	case extra := <-m.meters:
		t.Errorf("a second /meters request %v was sent for the same bank and rate", extra.args)
	case <-time.After(50 * time.Millisecond):
	}

	m.push(t, "/meters/1", int16MeterBlob(-3.5, MeterSilenceDB, 0))
	for i, ch := range []<-chan *MeterUpdate{first, second} {
		u := recvMeterUpdate(t, ch, time.Second)
		if u.Address != "/meters/1" || u.Bank != 1 {
			t.Errorf("subscriber %d: got %q (bank %d), want /meters/1 (bank 1)", i, u.Address, u.Bank)
		}
		if len(u.Values) != 3 || math.Abs(u.Values[0]-(-3.5)) > 1e-9 || u.Values[1] != MeterSilenceDB || u.Values[2] != 0 {
			t.Errorf("subscriber %d: values = %v, want [-3.5 %v 0]", i, u.Values, MeterSilenceDB)
		}
		if len(u.Blob) == 0 {
			t.Errorf("subscriber %d: raw blob was not passed through", i)
		}
	}
}

// TestSubscribeMetersSharesBankAtFastestRate covers what happens when
// two callers want the same bank at different rates: the mixer streams a
// bank to a socket at one rate, so they share a single subscription
// running at the faster of the two, and both see every block.
func TestSubscribeMetersSharesBankAtFastestRate(t *testing.T) {
	m := startPushMixer(t, nil)

	c, err := NewClient("127.0.0.1", m.port())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	slow, cancelSlow, err := c.SubscribeMeters(MeterRequest{Bank: 1, Interval: 500 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer cancelSlow()
	if req := m.waitMeters(t, time.Second); len(req.args) != 2 || req.args[1] != int32(10) {
		t.Fatalf("/meters request args = %v, want [/meters/1 10]", req.args)
	}

	fast, cancelFast, err := c.SubscribeMeters(MeterRequest{Bank: 1, Interval: 100 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer cancelFast()
	if req := m.waitMeters(t, time.Second); len(req.args) != 2 || req.args[1] != int32(2) {
		t.Fatalf("/meters request args = %v, want [/meters/1 2] once a faster subscriber joined", req.args)
	}

	m.push(t, "/meters/1", int16MeterBlob(-20))
	for i, ch := range []<-chan *MeterUpdate{slow, fast} {
		if u := recvMeterUpdate(t, ch, time.Second); len(u.Values) != 1 || u.Values[0] != -20 {
			t.Errorf("subscriber %d: values = %v, want [-20]", i, u.Values)
		}
	}
}

// TestSubscribeMetersSendsBankArguments covers the banks that select
// what they meter with their own arguments, which must precede the time
// factor (WIRE_PROTOCOL.md §1a).
func TestSubscribeMetersSendsBankArguments(t *testing.T) {
	m := startPushMixer(t, nil)

	c, err := NewClient("127.0.0.1", m.port())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	_, cancel, err := c.SubscribeMeters(MeterRequest{Bank: 5, Args: []int32{3, 1}, Interval: 200 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()

	req := m.waitMeters(t, time.Second)
	if len(req.args) != 4 || req.args[0] != "/meters/5" ||
		req.args[1] != int32(3) || req.args[2] != int32(1) || req.args[3] != int32(4) {
		t.Fatalf("/meters request args = %v, want [/meters/5 3 1 4]", req.args)
	}
}

// TestSubscribeMetersRenews checks that the client keeps the meter
// subscription alive on its own: the mixer drops it if no request
// arrives within ~10s (WIRE_PROTOCOL.md §1a).
func TestSubscribeMetersRenews(t *testing.T) {
	if DefaultMeterRenewInterval >= 10*time.Second {
		t.Errorf("DefaultMeterRenewInterval = %v, must stay under the mixer's ~10s subscription lifetime", DefaultMeterRenewInterval)
	}

	m := startPushMixer(t, nil)

	c, err := NewClient("127.0.0.1", m.port(), WithClientMeterRenewInterval(testMeterRenewInterval))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	_, cancel, err := c.SubscribeMeters(MeterRequest{Bank: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()

	prev := m.waitMeters(t, time.Second).at
	for i := 0; i < 3; i++ {
		next := m.waitMeters(t, time.Second).at
		if gap := next.Sub(prev); gap >= 10*time.Second {
			t.Errorf("renewal %d arrived %v after the previous /meters; the subscription would have lapsed", i, gap)
		}
		prev = next
	}
}

// TestUnsubscribeMetersStopsRenewal checks the other end of the
// lifecycle: dropping the last subscriber to a bank stops the ticker and
// lets the mixer-side subscription lapse by itself.
func TestUnsubscribeMetersStopsRenewal(t *testing.T) {
	m := startPushMixer(t, nil)

	c, err := NewClient("127.0.0.1", m.port(), WithClientMeterRenewInterval(testMeterRenewInterval))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	updates, cancel, err := c.SubscribeMeters(MeterRequest{Bank: 1})
	if err != nil {
		t.Fatal(err)
	}
	m.waitMeters(t, time.Second) // initial
	m.waitMeters(t, time.Second) // at least one renewal

	cancel()
	cancel() // idempotent

	if _, ok := <-updates; ok {
		t.Error("expected the meter channel to be closed after cancelling")
	}

	// A tick that was already in flight when cancel ran may still land,
	// so allow a grace period before requiring silence.
	grace := time.After(2 * testMeterRenewInterval)
draining:
	for {
		select {
		case <-m.meters:
		case <-grace:
			break draining
		}
	}
	select {
	case req := <-m.meters:
		t.Errorf("received /meters at %v, after the last subscriber went away", req.at)
	case <-time.After(10 * testMeterRenewInterval):
	}

	// A block still arriving during the ~10s the real subscription
	// takes to lapse must be dropped without panicking on the closed
	// channel.
	m.push(t, "/meters/1", int16MeterBlob(0))
	time.Sleep(50 * time.Millisecond)
}

// TestMetersDoNotDisturbGetOrPush locks in that meter blocks are routed
// to their own bank's subscribers only: request/reply is untouched, and
// a parameter-change subscriber is not handed a binary blob it has no
// use for.
func TestMetersDoNotDisturbGetOrPush(t *testing.T) {
	values := map[string]any{"/ch/01/mix/fader": float32(0.75)}
	m := startPushMixer(t, values)

	c, err := NewClient("127.0.0.1", m.port())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	updates, unsubscribe := c.Subscribe()
	defer unsubscribe()
	m.waitXRemote(t, time.Second)

	meters, cancel, err := c.SubscribeMeters(MeterRequest{Bank: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	m.waitMeters(t, time.Second)

	m.push(t, "/meters/1", int16MeterBlob(-10))
	if u := recvMeterUpdate(t, meters, time.Second); len(u.Values) != 1 || u.Values[0] != -10 {
		t.Errorf("meter values = %v, want [-10]", u.Values)
	}

	// The parameter-change subscriber sees the parameter push, and only
	// that: the meter block above went nowhere near it.
	m.push(t, "/ch/09/mix/on", int32(0))
	if u := recvUpdate(t, updates, time.Second); u.Address != "/ch/09/mix/on" {
		t.Fatalf("push subscriber saw %q first; meter blocks must not be published to it", u.Address)
	}

	args, err := c.GetTimeout("/ch/01/mix/fader")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(args) != 1 || args[0] != float32(0.75) {
		t.Fatalf("got %v, want [0.75]", args)
	}
}

// TestCloseClosesMeterChannels covers the shutdown path: a consumer
// ranging over its channel must see it end when the Client is closed.
func TestCloseClosesMeterChannels(t *testing.T) {
	m := startPushMixer(t, nil)

	c, err := NewClient("127.0.0.1", m.port(), WithClientMeterRenewInterval(testMeterRenewInterval))
	if err != nil {
		t.Fatal(err)
	}
	updates, cancel, err := c.SubscribeMeters(MeterRequest{Bank: 1})
	if err != nil {
		t.Fatal(err)
	}
	m.waitMeters(t, time.Second)

	c.Close()
	select {
	case _, ok := <-updates:
		if ok {
			t.Error("expected the meter channel to be closed by Close")
		}
	case <-time.After(time.Second):
		t.Error("Close did not close the meter channel")
	}
	cancel() // must not panic after Close

	// Subscribing to a closed Client hands back an already-closed
	// channel rather than one that could never deliver.
	after, cancelAfter, err := c.SubscribeMeters(MeterRequest{Bank: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer cancelAfter()
	if _, ok := <-after; ok {
		t.Error("expected SubscribeMeters on a closed Client to return a closed channel")
	}
}

func TestResolveMeter(t *testing.T) {
	cases := []struct {
		kindID KindID
		meter  Meter
		bank   int
		args   []int32
		index  int
	}{
		// X-Air puts every input, bus, send and output level in bank 1.
		{KindXR18, StripMeter(1), 1, nil, 0},
		{KindXR18, StripMeter(16), 1, nil, 15},
		{KindXR18, AuxReturnMeter(2), 1, nil, 17},
		{KindXR18, FXReturnMeter(1), 1, nil, 18},
		{KindXR18, BusMeter(6), 1, nil, 31},
		{KindXR18, FXSendMeter(1), 1, nil, 32},
		{KindXR18, MainMeter(1), 1, nil, 36},
		{KindXR18, MainMeter(2), 1, nil, 37},
		{KindXR18, MonitorMeter(2), 1, nil, 39},
		{KindXR18, DCAMeter(4), 8, nil, 3},
		// X32 spreads them over banks 0, 2 and 5, and interleaves
		// bank 9's sends with its returns.
		{KindX32, StripMeter(32), 0, nil, 31},
		{KindX32, AuxReturnMeter(1), 0, nil, 32},
		{KindX32, BusMeter(16), 0, nil, 63},
		{KindX32, MatrixMeter(6), 0, nil, 69},
		{KindX32, MainMeter(2), 2, nil, 23},
		{KindX32, MonoMeter(), 2, nil, 24},
		{KindX32, DCAMeter(1), 5, []int32{0, 0}, 16},
		{KindX32, FXSendMeter(2), 9, nil, 1},
		{KindX32, FXSendMeter(3), 9, nil, 4},
	}
	for _, c := range cases {
		p, err := resolveMeter(c.kindID, c.meter)
		if err != nil {
			t.Errorf("resolveMeter(%s, %s): unexpected error: %v", c.kindID, c.meter, err)
			continue
		}
		if p.bank != c.bank || p.index != c.index {
			t.Errorf("resolveMeter(%s, %s) = bank %d index %d, want bank %d index %d",
				c.kindID, c.meter, p.bank, p.index, c.bank, c.index)
		}
		if len(p.args) != len(c.args) {
			t.Errorf("resolveMeter(%s, %s) args = %v, want %v", c.kindID, c.meter, p.args, c.args)
		}
	}

	// A meter the family does not have, or a number outside the bank,
	// is refused rather than silently reading a neighboring value.
	for _, c := range []struct {
		kindID KindID
		meter  Meter
	}{
		{KindXR18, MatrixMeter(1)},
		{KindXR18, MonoMeter()},
		{KindXR18, StripMeter(17)},
		{KindXR18, StripMeter(0)},
		{KindX32, MonitorMeter(3)},
	} {
		if _, err := resolveMeter(c.kindID, c.meter); err == nil {
			t.Errorf("resolveMeter(%s, %s) = no error, want one", c.kindID, c.meter)
		}
	}
}

// TestSubscribeLevelsNamesMeters covers the layer callers are meant to
// use: naming the specific meters wanted and getting their levels, with
// no bank or index arithmetic.
func TestSubscribeLevelsNamesMeters(t *testing.T) {
	m := startPushMixer(t, nil)
	mixer := newTestMixer(t, KindXR18, m.port())

	frames, cancel, err := mixer.SubscribeLevels(
		[]Meter{StripMeter(1), StripMeter(16), MainMeter(1)},
		WithMeterInterval(100*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()

	// All three live in bank 1, so one subscription covers them.
	req := m.waitMeters(t, time.Second)
	if len(req.args) != 2 || req.args[0] != "/meters/1" || req.args[1] != int32(2) {
		t.Fatalf("/meters request args = %v, want [/meters/1 2]", req.args)
	}

	// A full bank-1 block: strip levels 1..16, then the rest, with the
	// main post-fader pair at indices 36 and 37.
	block := make([]float64, 40)
	for i := range block {
		block[i] = MeterSilenceDB
	}
	block[0] = -12.5
	block[15] = -3
	block[36] = -0.5
	m.push(t, "/meters/1", int16MeterBlob(block...))

	f := recvFrame(t, frames, time.Second)
	want := []float64{-12.5, -3, -0.5}
	if len(f.Levels) != len(want) {
		t.Fatalf("got %d levels, want %d", len(f.Levels), len(want))
	}
	for i, w := range want {
		if math.Abs(f.Levels[i]-w) > 1e-9 {
			t.Errorf("level %d = %v, want %v dB", i, f.Levels[i], w)
		}
	}
}

// TestSubscribeLevelsSpansBanks covers the X32, whose input strips and
// main outputs are in different banks: both are subscribed to, and
// frames are clocked off the bank the first meter asked for lives in.
func TestSubscribeLevelsSpansBanks(t *testing.T) {
	m := startPushMixer(t, nil)
	mixer := newTestMixer(t, KindX32, m.port())

	frames, cancel, err := mixer.SubscribeLevels([]Meter{StripMeter(1), MainMeter(1)})
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()

	banks := map[string]bool{}
	for i := 0; i < 2; i++ {
		req := m.waitMeters(t, time.Second)
		if len(req.args) != 2 {
			t.Fatalf("/meters request args = %v, want a bank and a time factor", req.args)
		}
		banks[req.args[0].(string)] = true
	}
	if !banks["/meters/0"] || !banks["/meters/2"] {
		t.Fatalf("subscribed to %v, want both /meters/0 and /meters/2", banks)
	}

	// The main block alone does not produce a frame: frames are clocked
	// off bank 0, which holds StripMeter(1).
	main := make([]float32, 49)
	main[22] = 0.5
	m.push(t, "/meters/2", float32MeterBlob(main...))
	select {
	case f := <-frames:
		t.Fatalf("got frame %v from the non-clock bank alone", f.Levels)
	case <-time.After(100 * time.Millisecond):
	}

	strips := make([]float32, 70)
	strips[0] = 1
	m.push(t, "/meters/0", float32MeterBlob(strips...))

	f := recvFrame(t, frames, time.Second)
	if len(f.Levels) != 2 {
		t.Fatalf("got %d levels, want 2", len(f.Levels))
	}
	if math.Abs(f.Levels[0]) > 1e-3 {
		t.Errorf("strip level = %v, want 0 dB", f.Levels[0])
	}
	if math.Abs(f.Levels[1]-(-6.0206)) > 1e-3 {
		t.Errorf("main level = %v, want -6.02 dB", f.Levels[1])
	}
}

// TestSubscribeLevelsRejectsUnavailableMeters checks that a meter this
// mixer does not have fails up front, before anything is subscribed to.
func TestSubscribeLevelsRejectsUnavailableMeters(t *testing.T) {
	m := startPushMixer(t, nil)
	mixer := newTestMixer(t, KindXR18, m.port())

	if _, _, err := mixer.SubscribeLevels(nil); err == nil {
		t.Error("expected an error for an empty meter list")
	}
	if _, _, err := mixer.SubscribeLevels([]Meter{StripMeter(1), MatrixMeter(1)}); err == nil {
		t.Error("expected an error for a matrix meter on an XR18")
	}
	select {
	case req := <-m.meters:
		t.Errorf("a /meters request %v was sent despite the failure", req.args)
	case <-time.After(50 * time.Millisecond):
	}
}

// TestSubscribeLevelsCancelClosesChannel covers the shutdown path of the
// named layer, including the fan-in goroutines it runs per bank.
func TestSubscribeLevelsCancelClosesChannel(t *testing.T) {
	m := startPushMixer(t, nil)
	mixer := newTestMixer(t, KindXR18, m.port(), WithClientMeterRenewInterval(testMeterRenewInterval))

	frames, cancel, err := mixer.SubscribeLevels([]Meter{StripMeter(1), DCAMeter(1)})
	if err != nil {
		t.Fatal(err)
	}
	m.waitMeters(t, time.Second)
	m.waitMeters(t, time.Second)

	cancel()
	cancel() // idempotent

	deadline := time.After(time.Second)
	for {
		select {
		case _, ok := <-frames:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("cancelling SubscribeLevels did not close the frame channel")
		}
	}
}
