package xair

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// testXRemoteInterval keeps the renewal tests fast; the shipping default
// is DefaultXRemoteInterval, asserted separately below.
const testXRemoteInterval = 30 * time.Millisecond

func recvUpdate(t *testing.T, ch <-chan *Update, timeout time.Duration) *Update {
	t.Helper()
	select {
	case u, ok := <-ch:
		if !ok {
			t.Fatal("update channel was closed while waiting for a push")
		}
		return u
	case <-time.After(timeout):
		t.Fatal("timed out waiting for a push update")
		return nil
	}
}

// TestSubscribeReceivesPushOnEverySubscriber covers the round trip: the
// client subscribes with /xremote, the mixer pushes an unsolicited
// parameter change to that socket, and every registered subscriber sees
// it with the value decoded into the same units the equivalent getter
// (here Mix.Fader) returns.
func TestSubscribeReceivesPushOnEverySubscriber(t *testing.T) {
	m := startPushMixer(t, nil)

	c, err := NewClient("127.0.0.1", m.port())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	first, unsubFirst := c.Subscribe()
	defer unsubFirst()
	second, unsubSecond := c.Subscribe()
	defer unsubSecond()

	m.waitXRemote(t, time.Second)
	m.push(t, "/ch/01/mix/fader", float32(0.5))

	for i, ch := range []<-chan *Update{first, second} {
		u := recvUpdate(t, ch, time.Second)
		if u.Address != "/ch/01/mix/fader" {
			t.Errorf("subscriber %d: address = %q, want /ch/01/mix/fader", i, u.Address)
		}
		if len(u.Args) != 1 || u.Args[0] != float32(0.5) {
			t.Errorf("subscriber %d: raw args = %v, want [0.5]", i, u.Args)
		}
		// 0.5 on the wire is -10 dB through the fader taper
		// (WIRE_PROTOCOL.md §6), the same as Mix.Fader would report.
		if db, ok := u.Value.(float64); !ok || db != -10 {
			t.Errorf("subscriber %d: decoded value = %v (%T), want -10 dB as float64", i, u.Value, u.Value)
		}
	}
}

// TestSubscribeRenewsXRemote checks that the client keeps the
// subscription alive on its own: the mixer drops it if no /xremote
// arrives within ~10s (WIRE_PROTOCOL.md §1).
func TestSubscribeRenewsXRemote(t *testing.T) {
	if DefaultXRemoteInterval >= 10*time.Second {
		t.Errorf("DefaultXRemoteInterval = %v, must stay under the mixer's ~10s subscription lifetime", DefaultXRemoteInterval)
	}

	m := startPushMixer(t, nil)

	c, err := NewClient("127.0.0.1", m.port(), WithClientXRemoteInterval(testXRemoteInterval))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	_, unsubscribe := c.Subscribe()
	defer unsubscribe()

	// The initial /xremote, then renewals that keep arriving without the
	// caller doing anything.
	prev := m.waitXRemote(t, time.Second)
	for i := 0; i < 3; i++ {
		next := m.waitXRemote(t, time.Second)
		if gap := next.Sub(prev); gap >= 10*time.Second {
			t.Errorf("renewal %d arrived %v after the previous /xremote; the subscription would have lapsed", i, gap)
		}
		prev = next
	}
}

// TestSubscribeDoesNotDisturbGet locks in that adding a subscriber only
// changes what happens to *unmatched* messages: replies to in-flight
// Gets still go to the goroutine that asked for them, and only the
// unsolicited push reaches the subscriber.
func TestSubscribeDoesNotDisturbGet(t *testing.T) {
	values := map[string]any{
		"/ch/01/mix/fader": float32(0.75),
		"/ch/02/mix/fader": float32(0.25),
		"/ch/03/mix/on":    int32(1),
		"/bus/01/mix/on":   int32(0),
		"/xinfo":           "10.0.0.1",
	}
	m := startPushMixer(t, values)

	c, err := NewClient("127.0.0.1", m.port())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	updates, unsubscribe := c.Subscribe()
	defer unsubscribe()
	m.waitXRemote(t, time.Second)

	var wg sync.WaitGroup
	errCh := make(chan error, len(values))
	for addr, want := range values {
		wg.Add(1)
		go func(addr string, want any) {
			defer wg.Done()
			args, err := c.GetTimeout(addr)
			if err != nil {
				errCh <- fmt.Errorf("%s: %w", addr, err)
				return
			}
			if len(args) != 1 || args[0] != want {
				errCh <- fmt.Errorf("%s: got %v, want [%v]", addr, args, want)
			}
		}(addr, want)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}

	// Every Get above has completed, so every reply has been dispatched.
	// The next thing the subscriber sees must be the unsolicited push,
	// not one of those replies.
	m.push(t, "/ch/09/mix/on", int32(0))
	u := recvUpdate(t, updates, time.Second)
	if u.Address != "/ch/09/mix/on" {
		t.Fatalf("subscriber saw %q first; matched replies must go to their waiter, not to subscribers", u.Address)
	}
	if on, ok := u.Value.(bool); !ok || on {
		t.Errorf("decoded value = %v (%T), want false as bool", u.Value, u.Value)
	}
	select {
	case extra := <-updates:
		t.Errorf("subscriber also received %q; Get replies must not be published", extra.Address)
	default:
	}

	// A Get issued after the push still works, i.e. publishing did not
	// leave the waiter map in a bad state.
	args, err := c.Get(context.Background(), "/ch/01/mix/fader")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(args) != 1 || args[0] != float32(0.75) {
		t.Fatalf("got %v, want [0.75]", args)
	}
}

// TestUnsubscribeStopsRenewal checks the other end of the lifecycle:
// dropping the last subscriber stops the ticker and lets the mixer-side
// subscription lapse by itself.
func TestUnsubscribeStopsRenewal(t *testing.T) {
	m := startPushMixer(t, nil)

	c, err := NewClient("127.0.0.1", m.port(), WithClientXRemoteInterval(testXRemoteInterval))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	updates, unsubscribe := c.Subscribe()
	m.waitXRemote(t, time.Second) // initial
	m.waitXRemote(t, time.Second) // at least one renewal

	unsubscribe()
	unsubscribe() // idempotent

	if _, ok := <-updates; ok {
		t.Error("expected the update channel to be closed after unsubscribing")
	}

	// A tick that was already in flight when unsubscribe ran may still
	// land, so allow a grace period before requiring silence.
	grace := time.After(2 * testXRemoteInterval)
draining:
	for {
		select {
		case <-m.xremote:
		case <-grace:
			break draining
		}
	}
	select {
	case ts := <-m.xremote:
		t.Errorf("received /xremote at %v, after the last subscriber went away", ts)
	case <-time.After(10 * testXRemoteInterval):
	}
}

// TestRenewalContinuesWhileASubscriberRemains checks that renewal is
// keyed on there being *any* subscriber, not on the one that started it.
func TestRenewalContinuesWhileASubscriberRemains(t *testing.T) {
	m := startPushMixer(t, nil)

	c, err := NewClient("127.0.0.1", m.port(), WithClientXRemoteInterval(testXRemoteInterval))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	_, unsubFirst := c.Subscribe()
	second, unsubSecond := c.Subscribe()
	defer unsubSecond()

	m.waitXRemote(t, time.Second)
	unsubFirst()

	m.waitXRemote(t, time.Second)
	m.push(t, "/lr/mix/fader", float32(1))
	if u := recvUpdate(t, second, time.Second); u.Address != "/lr/mix/fader" {
		t.Errorf("address = %q, want /lr/mix/fader", u.Address)
	}
}

// TestCloseClosesSubscriberChannels covers the shutdown path: a consumer
// ranging over its channel must see it end when the Client is closed.
func TestCloseClosesSubscriberChannels(t *testing.T) {
	m := startPushMixer(t, nil)

	c, err := NewClient("127.0.0.1", m.port(), WithClientXRemoteInterval(testXRemoteInterval))
	if err != nil {
		t.Fatal(err)
	}
	updates, unsubscribe := c.Subscribe()
	m.waitXRemote(t, time.Second)

	c.Close()
	select {
	case _, ok := <-updates:
		if ok {
			t.Error("expected the update channel to be closed by Close")
		}
	case <-time.After(time.Second):
		t.Error("Close did not close the update channel")
	}
	unsubscribe() // must not panic after Close

	// Subscribing to a closed Client hands back an already-closed
	// channel rather than one that could never deliver.
	after, unsubscribeAfter := c.Subscribe()
	defer unsubscribeAfter()
	if _, ok := <-after; ok {
		t.Error("expected Subscribe on a closed Client to return a closed channel")
	}
}
