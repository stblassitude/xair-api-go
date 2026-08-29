// Command basic mirrors the xair-api-python README example: connect to a
// mixer, rename an input strip, and unmute it.
package main

import (
	"context"
	"fmt"
	"log"

	xair "github.com/stblassitude/xair-api-go"
)

func main() {
	ctx := context.Background()
	mixer, err := xair.Connect(ctx, xair.KindXR18, "mixer.local")
	if err != nil {
		log.Fatal(err)
	}
	defer mixer.Close()

	strip := mixer.Strip[8]
	if err := strip.Config.SetName("sm7b"); err != nil {
		log.Fatal(err)
	}
	if err := strip.Mix.SetOn(true); err != nil {
		log.Fatal(err)
	}

	name, err := strip.Config.Name()
	if err != nil {
		log.Fatal(err)
	}
	on, err := strip.Mix.On()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("strip 09 (%s) on has been set to %v\n", name, on)
}
