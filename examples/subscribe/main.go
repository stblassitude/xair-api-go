// Command subscribe prints the mixer's unsolicited parameter-change
// pushes as they happen, instead of polling for them. It has no
// xair-api-python counterpart: that client never sends /xremote (see
// WIRE_PROTOCOL.md §1).
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"

	xair "github.com/stblassitude/xair-api-go"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	mixer, err := xair.Connect(ctx, xair.KindXR18, "mixer.local")
	if err != nil {
		log.Fatal(err)
	}
	defer mixer.Close()

	// The /xremote subscription is sent now and renewed automatically
	// until unsubscribe is called (or the mixer is closed).
	updates, unsubscribe := mixer.Subscribe()
	defer unsubscribe()

	fmt.Println("listening for changes; move a fader on the mixer (^C to stop)")
	for {
		select {
		case u, ok := <-updates:
			if !ok {
				return
			}
			// u.Value is the argument in the same units the matching
			// getter returns — dB for "/ch/NN/mix/fader", for instance —
			// or nil for an address this package has no conversion for,
			// in which case u.Args holds the raw OSC argument.
			if u.Value != nil {
				fmt.Printf("%s = %v\n", u.Address, u.Value)
			} else {
				fmt.Printf("%s = %v (raw)\n", u.Address, u.Args)
			}
		case <-ctx.Done():
			return
		}
	}
}
