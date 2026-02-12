package main

import (
	"fmt"
	"image/color"
	"math/rand/v2"
	"os"
	"os/signal"
	"syscall"

	"qrun/streamdeck"
)

func main() {
	dev, err := streamdeck.Open()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer dev.Close()

	fmt.Printf("Connected to: %s (serial: %s)\n", dev.Product(), dev.SerialNumber())

	dev.SetBrightness(80)

	for i := 0; i < streamdeck.KeyCount(); i++ {
		r := uint8(rand.IntN(256))
		g := uint8(rand.IntN(256))
		b := uint8(rand.IntN(256))
		dev.SetKeyColor(i, color.RGBA{r, g, b, 255})
	}

	keys := make(chan streamdeck.KeyEvent, 64)
	go func() {
		if err := dev.ReadKeys(keys); err != nil {
			fmt.Fprintf(os.Stderr, "Read error: %v\n", err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	for {
		select {
		case ev := <-keys:
			if ev.Pressed {
				fmt.Printf("Key %d pressed\n", ev.Key)
				r := uint8(rand.IntN(256))
				g := uint8(rand.IntN(256))
				b := uint8(rand.IntN(256))
				dev.SetKeyColor(ev.Key, color.RGBA{r, g, b, 255})
			} else {
				fmt.Printf("Key %d released\n", ev.Key)
			}
		case <-sig:
			fmt.Println()
			return
		}
	}
}
