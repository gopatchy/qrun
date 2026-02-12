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

func labelForKey(key int) string {
	row := key / streamdeck.KeyCols()
	col := key % streamdeck.KeyCols()
	switch row {
	case 0:
		return fmt.Sprintf("Ch %d\nSelect", col+1)
	case 1:
		return fmt.Sprintf("Ch %d\nMute", col+1)
	case 2:
		return fmt.Sprintf("Ch %d\nSolo", col+1)
	case 3:
		return fmt.Sprintf("Ch %d\nRec", col+1)
	}
	return fmt.Sprintf("Key %d", key)
}

func drawKey(dev *streamdeck.Device, key int, bg color.RGBA) {
	dim := color.RGBA{bg.R / 3, bg.G / 3, bg.B / 3, 255}
	dev.SetKeyText(key, dim, color.White, labelForKey(key))
}

func drawKeyActive(dev *streamdeck.Device, key int, bg color.RGBA) {
	dev.SetKeyText(key, bg, color.White, labelForKey(key))
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
		col := i % streamdeck.KeyCols()
		drawKey(dev, i, palette[col])
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
			col := ev.Key % streamdeck.KeyCols()
			active[ev.Key] = !active[ev.Key]
			if active[ev.Key] {
				drawKeyActive(dev, ev.Key, palette[col])
			} else {
				drawKey(dev, ev.Key, palette[col])
			}
			fmt.Printf("Key %d toggled %v\n", ev.Key, active[ev.Key])
		case <-sig:
			fmt.Println()
			return
		}
	}
}
