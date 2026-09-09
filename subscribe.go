package xair

import (
	"time"

	goosc "github.com/hypebeast/go-osc/osc"
)

// XRemoteAddress is the OSC address a client sends, with no arguments, to
// subscribe its own UDP source address/port to the mixer's unsolicited
// parameter-change push (WIRE_PROTOCOL.md §1).
const XRemoteAddress = "/xremote"

// updateBuffer is how many pushes a subscriber may fall behind by before
// further ones are dropped. Pushes arrive in bursts (moving one fader on
// the mixer's own surface emits a message per intermediate position), so
// this is sized to absorb a burst rather than a slow consumer.
const updateBuffer = 128

// Update is one unsolicited parameter-change message pushed by the mixer
// to a subscribed client (WIRE_PROTOCOL.md §1). Updates are shared by all
// subscribers and must be treated as read-only.
type Update struct {
	// Address is the full OSC address of the parameter that changed,
	// e.g. "/ch/01/mix/fader".
	Address string

	// Args holds the message's raw OSC arguments exactly as received —
	// int32, float32, or string (WIRE_PROTOCOL.md §6) — with the
	// normalized 0.0-1.0 wire value still un-decoded for float
	// parameters.
	Args []any

	// Value is Args[0] decoded into the same units the equivalent getter
	// on the node/mixin types returns: float64 dB for a fader or send
	// level, float64 Hz/ms/% for the lin/log-curve parameters, bool for
	// i-typed booleans, string for names and enumerated choices, int32
	// for plain integers.
	//
	// It is nil when this package has no conversion registered for the
	// address (an address outside the space in WIRE_PROTOCOL.md §8, or
	// one whose argument was not of the expected type); Address and Args
	// are always populated, so a caller can still map such a push itself.
	Value any
}

// subscription is one registered push consumer. closed guards against
// double-closing ch when both unsubscribe and Close run.
type subscription struct {
	ch     chan *Update
	closed bool
}

// Subscribe registers for the mixer's unsolicited parameter-change
// pushes and returns the channel they are delivered on plus a function
// that cancels the subscription.
//
// Registering the first subscriber immediately sends /xremote and starts
// re-sending it every DefaultXRemoteInterval (see
// WithClientXRemoteInterval) for as long as any subscriber remains; the
// caller does not have to renew anything. Cancelling the last
// subscription stops the renewal and lets the mixer-side subscription
// lapse on its own — there is no "unsubscribe" message in the protocol.
//
// Any number of subscribers may be registered at once; each receives
// every push. The returned channel is buffered and is closed by the
// cancel function or by Client.Close; once it is full, further pushes to
// that subscriber are dropped (with a warning logged) rather than
// stalling the receive loop for everyone else. The cancel function is
// idempotent.
//
// Only parameter-change pushes are delivered. Binary /meters/... blocks
// are a separate subscription with a wire format of its own
// (WIRE_PROTOCOL.md §1a); they go to their own subscribers, never here.
// See Client.SubscribeMeters and Mixer.SubscribeLevels.
func (c *Client) Subscribe() (<-chan *Update, func()) {
	s := &subscription{ch: make(chan *Update, updateBuffer)}

	c.subMu.Lock()
	// Checked under subMu, which Close takes only after closing done, so
	// a Subscribe racing a Close either registers in time to have its
	// channel closed by Close or takes this branch.
	select {
	case <-c.done:
		c.subMu.Unlock()
		close(s.ch)
		s.closed = true
		return s.ch, func() {}
	default:
	}
	c.subs = append(c.subs, s)
	first := c.stopXRemote == nil
	if first {
		stop := make(chan struct{})
		c.stopXRemote = stop
		go c.renewXRemote(stop)
	}
	c.subMu.Unlock()

	// Sent outside the lock, but before returning, so that by the time
	// the caller has the channel the mixer has been asked to push.
	if first {
		c.sendXRemote()
	}

	return s.ch, func() { c.unsubscribe(s) }
}

func (c *Client) unsubscribe(s *subscription) {
	c.subMu.Lock()
	defer c.subMu.Unlock()

	for i, existing := range c.subs {
		if existing == s {
			c.subs = append(c.subs[:i], c.subs[i+1:]...)
			break
		}
	}
	if !s.closed {
		close(s.ch)
		s.closed = true
	}
	if len(c.subs) == 0 {
		c.stopXRemoteLocked()
	}
}

func (c *Client) closeSubscriptions() {
	c.subMu.Lock()
	defer c.subMu.Unlock()

	for _, s := range c.subs {
		if !s.closed {
			close(s.ch)
			s.closed = true
		}
	}
	c.subs = nil
	c.stopXRemoteLocked()
}

// stopXRemoteLocked ends the renewal goroutine, if one is running. The
// caller must hold subMu.
func (c *Client) stopXRemoteLocked() {
	if c.stopXRemote != nil {
		close(c.stopXRemote)
		c.stopXRemote = nil
	}
}

// renewXRemote re-subscribes on a ticker until the last subscriber goes
// away or the Client is closed. The mixer forgets a subscription that is
// not renewed within ~10s (WIRE_PROTOCOL.md §1).
func (c *Client) renewXRemote(stop <-chan struct{}) {
	t := time.NewTicker(c.xremoteInterval)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			c.sendXRemote()
		case <-stop:
			return
		case <-c.done:
			return
		}
	}
}

func (c *Client) sendXRemote() {
	if err := c.Send(XRemoteAddress); err != nil {
		c.logger.Warn("xair: could not send /xremote subscription renewal", "err", err)
	}
}

// publish offers an unmatched incoming message to every subscriber. With
// no subscribers registered the message is dropped, which is the
// pre-subscription behavior for anything that is not a reply somebody is
// waiting for.
func (c *Client) publish(msg *goosc.Message) {
	c.subMu.Lock()
	defer c.subMu.Unlock()

	if len(c.subs) == 0 {
		return
	}
	u := &Update{
		Address: msg.Address,
		Args:    msg.Arguments,
		Value:   decodePush(msg.Address, msg.Arguments),
	}
	// Sent under subMu (never blocking, since the channel is buffered
	// and the send is non-blocking) so that a subscriber cannot be
	// closed out from under an in-flight delivery.
	for _, s := range c.subs {
		select {
		case s.ch <- u:
		default:
			c.logger.Warn("xair: dropping push update, subscriber is not keeping up", "addr", msg.Address)
		}
	}
}
