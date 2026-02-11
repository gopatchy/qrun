package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"gitlab.com/gomidi/midi/v2"
	_ "gitlab.com/gomidi/midi/v2/drivers/rtmididrv"

	"qrun/xtouch"
)

func main() {
	defer midi.CloseDriver()

	port, err := xtouch.FindInPort("x-touch")
	if err != nil {
		fmt.Println("Available MIDI input ports:")
		for _, p := range midi.GetInPorts() {
			fmt.Printf("  %s\n", p)
		}
		fmt.Fprintf(os.Stderr, "\nError: %v\n", err)
		os.Exit(1)
	}

	dec := &xtouch.Decoder{EncoderMode: xtouch.EncoderRelative}

	fmt.Printf("Listening on: %s\n", port)

	stop, err := midi.ListenTo(port, func(msg midi.Message, timestampms int32) {
		if event := dec.Decode(msg); event != nil {
			fmt.Println(event)
		}
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listening: %v\n", err)
		os.Exit(1)
	}
	defer stop()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	fmt.Println()
}
