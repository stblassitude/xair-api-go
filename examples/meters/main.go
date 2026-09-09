// Command meters prints the mixer's live input levels, driven by the
// /meters subscription rather than by polling. It has no
// xair-api-python counterpart: that client never subscribes to meter
// blocks (see WIRE_PROTOCOL.md §1a).
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"time"

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

	// Name the meters wanted; SubscribeLevels works out which /meters
	// bank each one lives in for this mixer kind, subscribes, and keeps
	// the subscription renewed until cancel is called (or the mixer is
	// closed).
	meters := []xair.Meter{}
	for i := 1; i <= 8; i++ {
		meters = append(meters, xair.StripMeter(i))
	}
	meters = append(meters, xair.MainMeter(1), xair.MainMeter(2))

	// 10 Hz is plenty for a level display and asks the mixer for a fifth
	// of the datagrams its 50 ms floor would.
	levels, cancel, err := mixer.SubscribeLevels(meters, xair.WithMeterInterval(100*time.Millisecond))
	if err != nil {
		log.Fatal(err)
	}
	defer cancel()

	fmt.Println("metering channels 1-8 and the main output (^C to stop)")
	for {
		select {
		case frame, ok := <-levels:
			if !ok {
				return
			}
			// frame.Levels is in dBFS, in the order the meters were
			// asked for, floored at xair.MeterSilenceDB for silence.
			var b strings.Builder
			for i, db := range frame.Levels {
				fmt.Fprintf(&b, " %s%5.1f", label(meters[i]), db)
			}
			fmt.Printf("\r%s", b.String())
		case <-ctx.Done():
			fmt.Println()
			return
		}
	}
}

func label(m xair.Meter) string {
	if m.Source == xair.MeterMain {
		return []string{"L", "R"}[m.Number-1] + ":"
	}
	return fmt.Sprintf("%d:", m.Number)
}
