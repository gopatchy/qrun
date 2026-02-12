package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"os"
	"os/signal"
	"syscall"

	"qrun/lib/streamdeck"
)

func clamp(v int) int {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return v
}

func main() {
	dev, err := streamdeck.OpenModel(&streamdeck.ModelPlus)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer dev.Close()

	dev.SetBrightness(80)

	rgb := [3]int{0, 0, 0}
	fine := [3]bool{false, false, false}
	labels := [3]string{"R", "G", "B"}
	labelColors := [3]color.RGBA{
		{255, 0, 0, 255},
		{0, 255, 0, 255},
		{0, 100, 255, 255},
	}

	updateLCD := func() {
		c := color.RGBA{uint8(rgb[0]), uint8(rgb[1]), uint8(rgb[2]), 255}
		dev.SetLCDColor(0, 0, dev.Model().LCDWidth, dev.Model().LCDHeight, c)
	}

	updateKey := func(i int) {
		bg := color.RGBA{labelColors[i].R / 4, labelColors[i].G / 4, labelColors[i].B / 4, 255}
		sz := dev.Model().KeySize
		txt := streamdeck.TextImageWithFaceSized(streamdeck.MonoBoldSmall, sz, bg, labelColors[i], labels[i], fmt.Sprintf("%d", rgb[i]))
		if fine[i] {
			img := image.NewRGBA(image.Rect(0, 0, sz, sz))
			draw.Draw(img, img.Bounds(), txt, image.Point{}, draw.Src)
			border := labelColors[i]
			b := 4
			for y := 0; y < sz; y++ {
				for x := 0; x < sz; x++ {
					if x < b || x >= sz-b || y < b || y >= sz-b {
						img.Set(x, y, border)
					}
				}
			}
			dev.SetKeyImage(i, img)
		} else {
			dev.SetKeyImage(i, txt)
		}
	}

	updateAllKeys := func() {
		for i := 0; i < 3; i++ {
			updateKey(i)
		}
	}

	updateLCD()
	updateAllKeys()

	for i := 3; i < dev.Model().Keys; i++ {
		dev.ClearKey(i)
	}

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
			if ev.Encoder != nil && ev.Encoder.Encoder < 3 {
				i := ev.Encoder.Encoder
				if ev.Encoder.Delta != 0 {
					delta := ev.Encoder.Delta
					if !fine[i] {
						delta *= 10
					}
					rgb[i] = clamp(rgb[i] + delta)
					updateLCD()
					updateKey(i)
					fmt.Printf("R=%d G=%d B=%d\n", rgb[0], rgb[1], rgb[2])
				} else if ev.Encoder.Pressed {
					fine[i] = !fine[i]
					updateKey(i)
				}
			}
		case <-sig:
			fmt.Println()
			return
		}
	}
}
