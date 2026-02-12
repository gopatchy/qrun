package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/image/font"
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

	stripFont := streamdeck.LoadFace("fonts/AtkinsonHyperlegibleMono-Bold.ttf", 28)

	rgb := [3]int{0, 0, 0}
	hiNibble := [3]bool{false, false, false}
	names := [3]string{"Red", "Green", "Blue"}
	chanColors := [3]color.RGBA{
		{255, 80, 80, 255},
		{80, 255, 80, 255},
		{80, 160, 255, 255},
	}

	m := dev.Model()
	segW := m.LCDWidth / 4
	half := m.LCDHeight / 2

	updateLCD := func() {
		bg := color.RGBA{uint8(rgb[0]), uint8(rgb[1]), uint8(rgb[2]), 255}
		img := image.NewRGBA(image.Rect(0, 0, m.LCDWidth, m.LCDHeight))
		draw.Draw(img, img.Bounds(), &image.Uniform{bg}, image.Point{}, draw.Src)
		for i := 0; i < 3; i++ {
			x0 := i * segW
			x1 := (i + 1) * segW
			cc := chanColors[i]

			top := img.SubImage(image.Rect(x0, 0, x1, half)).(*image.RGBA)
			streamdeck.DrawOutlinedText(top, stripFont, cc, color.Black, 2, names[i])

			valStr := fmt.Sprintf("%03d %02x", rgb[i], rgb[i])
			bot := img.SubImage(image.Rect(x0, half, x1, m.LCDHeight)).(*image.RGBA)
			streamdeck.DrawOutlinedText(bot, stripFont, cc, color.Black, 2, valStr)

			charW := font.MeasureString(stripFont, "0").Ceil()
			fullW := font.MeasureString(stripFont, valStr).Ceil()
			metrics := stripFont.Metrics()
			lineH := metrics.Height.Ceil()
			botH := m.LCDHeight - half
			textX := x0 + (segW-fullW)/2
			baseY := half + (botH-lineH)/2 + metrics.Ascent.Ceil() + 2

			nibbleIdx := 5
			if hiNibble[i] {
				nibbleIdx = 4
			}
			ux := textX + nibbleIdx*charW
			for x := ux; x < ux+charW; x++ {
				img.Set(x, baseY, cc)
			}
		}
		dev.SetLCDImage(0, 0, m.LCDWidth, m.LCDHeight, img)
	}

	updateLCD()

	for i := 0; i < m.Keys; i++ {
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
					step := 1
					if hiNibble[i] {
						step = 16
					}
					rgb[i] = clamp(rgb[i] + ev.Encoder.Delta*step)
					updateLCD()
					fmt.Printf("R=%d G=%d B=%d\n", rgb[0], rgb[1], rgb[2])
				} else if ev.Encoder.Pressed {
					hiNibble[i] = !hiNibble[i]
					updateLCD()
				}
			}
		case <-sig:
			fmt.Println()
			return
		}
	}
}
