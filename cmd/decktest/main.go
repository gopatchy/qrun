package main

import (
	"fmt"
	"image/color"
	"os"
	"os/signal"
	"syscall"

	"qrun/lib/streamdeck"
)

var keyLabels = []string{
	"1", "2", "3", "4", "5", "6", "7", "8",
	"A", "B", "C", "D", "E", "F", "G", "H",
	"I", "J", "K", "L", "M", "N", "O", "P",
	"Q", "R", "S", "T", "U", "V", "W", "X",
}

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

func drawKey(dev *streamdeck.Device, key int, active bool) {
	col := key % streamdeck.KeyCols()
	bg := palette[col]
	if !active {
		bg = color.RGBA{bg.R / 3, bg.G / 3, bg.B / 3, 255}
	}
	dev.SetKeyText(key, bg, color.White, keyLabels[key])
}

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
		drawKey(dev, i, false)
	}

	active := make([]bool, streamdeck.KeyCount())

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
			if !ev.Pressed {
				continue
			}
			active[ev.Key] = !active[ev.Key]
			drawKey(dev, ev.Key, active[ev.Key])
			fmt.Printf("Key %s toggled %v\n", keyLabels[ev.Key], active[ev.Key])
		case <-sig:
			fmt.Println()
			return
		}
	}
}
