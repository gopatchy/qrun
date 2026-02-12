package main

import (
	"fmt"
	"image/color"
	"os"
	"os/signal"
	"syscall"

	"qrun/lib/streamdeck"
)

var palette = []color.RGBA{
	{220, 50, 50, 255},
	{50, 180, 50, 255},
	{50, 100, 220, 255},
	{220, 160, 30, 255},
	{180, 50, 180, 255},
	{50, 180, 180, 255},
	{220, 120, 50, 255},
	{100, 100, 200, 255},
}

func main() {
	dev, err := streamdeck.Open()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer dev.Close()

	model := dev.Model()
	fmt.Printf("Connected to: %s (%s, serial: %s)\n", dev.Product(), model.Name, dev.SerialNumber())
	fmt.Printf("Keys: %d (%dx%d), Encoders: %d\n", model.Keys, model.KeyCols, model.KeyRows, model.Encoders)

	dev.SetBrightness(80)

	keyLabels := make([]string, model.Keys)
	for i := range keyLabels {
		if i < 8 {
			keyLabels[i] = string(rune('1' + i))
		} else {
			keyLabels[i] = string(rune('A' + i - 8))
		}
	}

	drawKey := func(key int, active bool) {
		col := key % model.KeyCols
		bg := palette[col%len(palette)]
		if !active {
			bg = color.RGBA{bg.R / 3, bg.G / 3, bg.B / 3, 255}
		}
		dev.SetKeyText(key, bg, color.White, keyLabels[key])
	}

	for i := 0; i < model.Keys; i++ {
		drawKey(i, false)
	}

	if model.LCDWidth > 0 {
		dev.SetLCDColor(0, 0, model.LCDWidth, model.LCDHeight, color.RGBA{30, 30, 30, 255})
	}

	active := make([]bool, model.Keys)

	input := make(chan streamdeck.InputEvent, 64)
	go func() {
		if err := dev.ReadInput(input); err != nil {
			fmt.Fprintf(os.Stderr, "Read error: %v\n", err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	for {
		select {
		case ev := <-input:
			if ev.Key != nil && ev.Key.Pressed {
				k := ev.Key.Key
				active[k] = !active[k]
				drawKey(k, active[k])
				fmt.Printf("Key %s toggled %v\n", keyLabels[k], active[k])
			}
			if ev.Encoder != nil {
				e := ev.Encoder
				if e.Delta != 0 {
					fmt.Printf("Encoder %d rotated %+d\n", e.Encoder, e.Delta)
				} else {
					fmt.Printf("Encoder %d pressed=%v\n", e.Encoder, e.Pressed)
				}
			}
			if ev.Touch != nil {
				t := ev.Touch
				if t.Type == streamdeck.TouchSwipe {
					fmt.Printf("Touch swipe (%d,%d) -> (%d,%d)\n", t.X, t.Y, t.X2, t.Y2)
				} else {
					fmt.Printf("Touch %v at (%d,%d)\n", t.Type, t.X, t.Y)
				}
			}
		case <-sig:
			fmt.Println()
			return
		}
	}
}
