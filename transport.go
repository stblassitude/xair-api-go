package xair

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	goosc "github.com/hypebeast/go-osc/osc"
)

// DefaultRequestTimeout bounds how long Get waits for a reply before
// giving up. This replaces xair-api-python's fixed post-send sleep
// (WIRE_PROTOCOL.md §4): because Get returns as soon as the matching
// reply arrives, the timeout can safely be generous without adding
// latency to the common case.
const DefaultRequestTimeout = 500 * time.Millisecond

// DefaultXRemoteInterval is how often a Client with at least one active
// subscriber re-sends /xremote. The mixer drops a subscription that is
// not renewed within ~10s (WIRE_PROTOCOL.md §1), so this is deliberately
// well under that bound.
const DefaultXRemoteInterval = 8 * time.Second

// Client is a synchronous OSC client for a single mixer connection.
//
// It deliberately does not use hypebeast/go-osc's own Client/Server
// split: that Client dials a fresh, short-lived UDP socket for every
// Send and closes it again immediately, while hypebeast/go-osc's Server
// expects a separate, permanently bound socket for receiving. These
// mixers reply to the source port of the request itself, not to some
// other fixed "server" port, so those two would never see each other's
// traffic. Client instead keeps one UDP socket connected to the mixer
// for both directions (matching xair-api-python's single-socket design),
// and dispatches replies by OSC address to per-request channels rather
// than xair-api-python's single "last packet wins" field — see
// WIRE_PROTOCOL.md §4 for the race that avoids.
type Client struct {
	conn *net.UDPConn
	ip   string
	port int

	logger         *slog.Logger
	requestTimeout time.Duration

	mu      sync.Mutex
	waiters map[string][]chan *goosc.Message

	xremoteInterval time.Duration

	subMu       sync.Mutex
	subs        []*subscription
	stopXRemote chan struct{}

	meterEncoding      MeterEncoding
	meterRenewInterval time.Duration

	meterMu     sync.Mutex
	meterGroups map[string]*meterGroup

	closeOnce sync.Once
	done      chan struct{}
}

// ClientOption configures a Client created by NewClient.
type ClientOption func(*Client)

// WithClientLogger sets the logger used for transport-level warnings.
// Defaults to slog.Default().
func WithClientLogger(l *slog.Logger) ClientOption { return func(c *Client) { c.logger = l } }

// WithClientRequestTimeout sets how long Get waits for a reply. Defaults
// to DefaultRequestTimeout.
func WithClientRequestTimeout(d time.Duration) ClientOption {
	return func(c *Client) { c.requestTimeout = d }
}

// WithClientXRemoteInterval sets how often /xremote is re-sent while at
// least one subscriber is registered. Defaults to DefaultXRemoteInterval;
// values of 10s or more will let the subscription lapse between renewals.
func WithClientXRemoteInterval(d time.Duration) ClientOption {
	return func(c *Client) { c.xremoteInterval = d }
}

// WithClientMeterRenewInterval sets how often a meter bank's /meters
// request is re-sent while at least one subscriber to it is registered.
// Defaults to DefaultMeterRenewInterval; values of 10s or more will let
// the subscription lapse between renewals.
func WithClientMeterRenewInterval(d time.Duration) ClientOption {
	return func(c *Client) { c.meterRenewInterval = d }
}

// WithClientMeterEncoding sets how the values inside a meter block are
// encoded, which differs by mixer family (WIRE_PROTOCOL.md §1a).
// Defaults to MeterEncodingInt16DB, the X-Air/MR encoding, matching this
// constructor's X-Air-oriented defaults; Connect sets it from the
// KindID instead.
func WithClientMeterEncoding(e MeterEncoding) ClientOption {
	return func(c *Client) { c.meterEncoding = e }
}

// NewClient opens a UDP socket connected to ip:port and starts listening
// for replies from it. Call Close when done.
func NewClient(ip string, port int, opts ...ClientOption) (*Client, error) {
	raddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", ip, port))
	if err != nil {
		return nil, err
	}
	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		return nil, err
	}

	c := &Client{
		conn:            conn,
		ip:              ip,
		port:            port,
		logger:          slog.Default(),
		requestTimeout:  DefaultRequestTimeout,
		xremoteInterval: DefaultXRemoteInterval,
		waiters:         make(map[string][]chan *goosc.Message),
		done:            make(chan struct{}),

		meterEncoding:      MeterEncodingInt16DB,
		meterRenewInterval: DefaultMeterRenewInterval,
		meterGroups:        make(map[string]*meterGroup),
	}
	for _, opt := range opts {
		opt(c)
	}

	go c.readLoop()
	return c, nil
}

// Close stops the receive loop, stops any /xremote or /meters renewal,
// closes every subscriber channel handed out by Subscribe and
// SubscribeMeters, and closes the underlying socket.
func (c *Client) Close() error {
	c.closeOnce.Do(func() { close(c.done) })
	c.closeSubscriptions()
	c.closeMeterSubscriptions()
	return c.conn.Close()
}

func (c *Client) readLoop() {
	buf := make([]byte, 65535)
	for {
		n, err := c.conn.Read(buf)
		if err != nil {
			select {
			case <-c.done:
			default:
				c.logger.Warn("xair: reply socket closed unexpectedly", "err", err)
			}
			return
		}

		packet, err := goosc.ParsePacket(string(buf[:n]))
		if err != nil {
			c.logger.Warn("xair: could not parse incoming OSC packet", "err", err)
			continue
		}
		msg, ok := packet.(*goosc.Message)
		if !ok {
			// Bundles are never sent by these mixers (WIRE_PROTOCOL.md §2);
			// ignore defensively rather than guess how to route one.
			continue
		}
		c.dispatch(msg)
	}
}

func (c *Client) dispatch(msg *goosc.Message) {
	c.mu.Lock()
	var next chan *goosc.Message
	if chans := c.waiters[msg.Address]; len(chans) > 0 {
		next = chans[0]
		if len(chans) == 1 {
			delete(c.waiters, msg.Address)
		} else {
			c.waiters[msg.Address] = chans[1:]
		}
	}
	c.mu.Unlock()

	if next != nil {
		next <- msg
		return
	}
	// Nobody is waiting on this address, so this is not a reply to any
	// in-flight request: it is a binary meter block from a /meters
	// subscription (WIRE_PROTOCOL.md §1a), an unsolicited push from an
	// /xremote subscription (§1), or a late reply whose Get has already
	// timed out. Meter blocks go to the subscribers of their own bank
	// and no further, since their blob argument means nothing to a
	// parameter-change consumer; everything else is offered to the push
	// subscribers. With nobody registered, both paths drop the message
	// rather than clobbering some unrelated in-flight request's result,
	// unlike xair-api-python's single shared response field.
	if c.dispatchMeter(msg) {
		return
	}
	c.publish(msg)
}

func (c *Client) register(addr string) chan *goosc.Message {
	ch := make(chan *goosc.Message, 1)
	c.mu.Lock()
	c.waiters[addr] = append(c.waiters[addr], ch)
	c.mu.Unlock()
	return ch
}

func (c *Client) unregister(addr string, ch chan *goosc.Message) {
	c.mu.Lock()
	defer c.mu.Unlock()
	chans := c.waiters[addr]
	for i, existing := range chans {
		if existing == ch {
			c.waiters[addr] = append(chans[:i], chans[i+1:]...)
			break
		}
	}
	if len(c.waiters[addr]) == 0 {
		delete(c.waiters, addr)
	}
}

// Send fire-and-forgets an OSC command: addr with no args is a GET
// request, addr with one argument is a SET (WIRE_PROTOCOL.md §3).
// Arguments must already be OSC-wire-typed (int32, float32, or string —
// WIRE_PROTOCOL.md §6). Booleans are sent as int32 0/1, never as OSC's
// native True/False types, since that is what these mixers expect.
func (c *Client) Send(addr string, args ...any) error {
	msg := goosc.NewMessage(addr, args...)
	data, err := msg.MarshalBinary()
	if err != nil {
		return err
	}
	_, err = c.conn.Write(data)
	return err
}

// Get sends a GET for addr and waits for the mixer's reply to that exact
// address, or until ctx is done. It returns the reply's argument list.
func (c *Client) Get(ctx context.Context, addr string) ([]any, error) {
	ch := c.register(addr)
	if err := c.Send(addr); err != nil {
		c.unregister(addr, ch)
		return nil, err
	}
	select {
	case msg := <-ch:
		return msg.Arguments, nil
	case <-ctx.Done():
		c.unregister(addr, ch)
		return nil, fmt.Errorf("xair: get %s: %w", addr, ctx.Err())
	}
}

// GetTimeout is Get with the Client's configured request timeout applied.
func (c *Client) GetTimeout(addr string) ([]any, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.requestTimeout)
	defer cancel()
	return c.Get(ctx, addr)
}
