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

var lcdColors = []xtouch.LCDColor{
	xtouch.ColorRed,
	xtouch.ColorGreen,
	xtouch.ColorYellow,
	xtouch.ColorBlue,
	xtouch.ColorMagenta,
	xtouch.ColorCyan,
	xtouch.ColorWhite,
}

var (
	faderValues   [9]uint8
	encoderValues [8]int
)

func updateLCD(out *xtouch.Output, ch uint8) {
	color := lcdColors[encoderValues[ch]*len(lcdColors)/128]
	out.SetLCD(ch, color, false, false,
		fmt.Sprintf("E%-3d F%-3d", encoderValues[ch], faderValues[ch]),
		fmt.Sprintf("Chan %d", ch+1))
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func main() {
	defer midi.CloseDriver()

	inPort, err := xtouch.FindInPort("x-touch")
	if err != nil {
		fmt.Println("Available MIDI input ports:")
		for _, p := range midi.GetInPorts() {
			fmt.Printf("  %s\n", p)
		}
		fmt.Fprintf(os.Stderr, "\nError: %v\n", err)
		os.Exit(1)
	}

	outPort, err := xtouch.FindOutPort("x-touch")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	out, err := xtouch.NewOutput(outPort, xtouch.DeviceIDExtender)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	for i := uint8(0); i < 8; i++ {
		updateLCD(out, i)
	}

	dec := &xtouch.Decoder{EncoderMode: xtouch.EncoderRelative}

	fmt.Printf("Listening on: %s\n", inPort)

	stop, err := midi.ListenTo(inPort, func(msg midi.Message, timestampms int32) {
		event := dec.Decode(msg)
		if event == nil {
			return
		}
		fmt.Println(event)

		switch e := event.(type) {
		case xtouch.FaderEvent:
			if e.Fader > 7 {
				return
			}
			faderValues[e.Fader] = e.Value
			pair := e.Fader ^ 1
			faderValues[pair] = e.Value
			out.SetFader(pair, e.Value)
			out.SetMeter(e.Fader, e.Value)
			out.SetMeter(pair, e.Value)
			updateLCD(out, e.Fader)
			updateLCD(out, pair)

		case xtouch.EncoderRelativeEvent:
			encoderValues[e.Encoder] = clamp(encoderValues[e.Encoder]+e.Delta, 0, 127)
			out.SetEncoderRing(e.Encoder, uint8(encoderValues[e.Encoder]))
			updateLCD(out, e.Encoder)
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
