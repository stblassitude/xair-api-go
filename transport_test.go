package xair

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	goosc "github.com/hypebeast/go-osc/osc"
)

// startFakeMixer answers each incoming GET (an address with no arguments)
// found in values by replying to the sender with a message to that same
// address. Replies are deliberately staggered by a delay that varies per
// address, so they arrive interleaved rather than in request order — this
// is what defeats xair-api-python's "whatever arrived most recently, on
// any address" read (WIRE_PROTOCOL.md §4) but must not defeat Client,
// which routes each reply to the specific request that asked for it.
func startFakeMixer(t *testing.T, values map[string]any) *net.UDPConn {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		buf := make([]byte, 65535)
		for {
			n, addr, err := conn.ReadFromUDP(buf)
			if err != nil {
				return
			}
			packet, err := goosc.ParsePacket(string(buf[:n]))
			if err != nil {
				continue
			}
			msg, ok := packet.(*goosc.Message)
			if !ok {
				continue
			}
			val, known := values[msg.Address]
			if !known {
				continue
			}
			go func(peer *net.UDPAddr, address string, v any) {
				time.Sleep(time.Duration(len(address)%5) * time.Millisecond)
				reply := goosc.NewMessage(address, v)
				data, err := reply.MarshalBinary()
				if err != nil {
					return
				}
				_, _ = conn.WriteToUDP(data, peer)
			}(addr, msg.Address, val)
		}
	}()
	return conn
}

func TestClientGetRoutesRepliesByAddressConcurrently(t *testing.T) {
	values := map[string]any{
		"/ch/01/mix/fader": float32(0.75),
		"/ch/02/mix/fader": float32(0.25),
		"/ch/03/mix/on":    int32(1),
		"/bus/01/mix/on":   int32(0),
		"/xinfo":           "10.0.0.1",
	}
	mixer := startFakeMixer(t, values)
	defer mixer.Close()

	laddr := mixer.LocalAddr().(*net.UDPAddr)
	c, err := NewClient("127.0.0.1", laddr.Port)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	var wg sync.WaitGroup
	errCh := make(chan error, len(values))
	for addr, want := range values {
		wg.Add(1)
		go func(addr string, want any) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			args, err := c.Get(ctx, addr)
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
}

func TestClientGetTimesOutWithoutCorruptingLaterRequests(t *testing.T) {
	values := map[string]any{
		"/ch/01/mix/fader": float32(0.5),
	}
	mixer := startFakeMixer(t, values)
	defer mixer.Close()

	laddr := mixer.LocalAddr().(*net.UDPAddr)
	c, err := NewClient("127.0.0.1", laddr.Port)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	// This address has no reply configured, so it must time out...
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := c.Get(ctx, "/ch/99/mix/fader"); err == nil {
		t.Fatal("expected a timeout error for an address the fake mixer never answers")
	}

	// ...without leaving stale state that corrupts a subsequent, unrelated
	// request the way xair-api-python's single shared response field
	// would (WIRE_PROTOCOL.md §4).
	args, err := c.Get(context.Background(), "/ch/01/mix/fader")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(args) != 1 || args[0] != float32(0.5) {
		t.Fatalf("got %v, want [0.5]", args)
	}
}
