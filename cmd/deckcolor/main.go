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

type channel struct {
	Name    string
	Color   color.RGBA
	Primary color.RGBA
	Value   int
}

func mixColor(channels []channel) color.RGBA {
	var r, g, b int
	for i, ch := range channels {
		if i == 0 {
			continue
		}
		f := ch.Value
		r += int(ch.Primary.R) * f / 255
		g += int(ch.Primary.G) * f / 255
		b += int(ch.Primary.B) * f / 255
	}
	intensity := channels[0].Value
	r = r * intensity / 255
	g = g * intensity / 255
	b = b * intensity / 255
	return color.RGBA{uint8(clamp(r)), uint8(clamp(g)), uint8(clamp(b)), 255}
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

	channels := []channel{
		{"Int", color.RGBA{220, 220, 220, 255}, color.RGBA{255, 255, 255, 255}, 0},
		{"Red", color.RGBA{255, 80, 80, 255}, color.RGBA{255, 0, 0, 255}, 0},
		{"Blue", color.RGBA{80, 120, 255, 255}, color.RGBA{0, 0, 255, 255}, 0},
		{"Green", color.RGBA{80, 255, 80, 255}, color.RGBA{0, 255, 0, 255}, 0},
		{"Amber", color.RGBA{255, 200, 60, 255}, color.RGBA{255, 191, 0, 255}, 0},
		{"Lime", color.RGBA{200, 255, 60, 255}, color.RGBA{191, 255, 0, 255}, 0},
		{"Cyan", color.RGBA{60, 255, 255, 255}, color.RGBA{0, 255, 255, 255}, 0},
		{"White", color.RGBA{220, 220, 220, 255}, color.RGBA{255, 255, 255, 255}, 0},
	}

	hiNibble := make([]bool, len(channels))
	for i := range hiNibble {
		hiNibble[i] = true
	}
	page := 0

	pages := []struct {
		Label string
		Start int
		End   int
	}{
		{"IRBG", 0, 4},
		{"ALCW", 4, 8},
	}

	m := dev.Model()
	half := m.LCDHeight / 2

	updateLCD := func() {
		p := pages[page]
		count := p.End - p.Start
		segW := m.LCDWidth / m.Encoders
		bg := mixColor(channels)
		img := image.NewRGBA(image.Rect(0, 0, m.LCDWidth, m.LCDHeight))
		draw.Draw(img, img.Bounds(), &image.Uniform{bg}, image.Point{}, draw.Src)
		charW := font.MeasureString(stripFont, "0").Ceil()
		pad := 8
		for e := 0; e < count; e++ {
			ci := p.Start + e
			ch := channels[ci]
			cc := ch.Color
			mid := e*segW + segW/2

			valStr := fmt.Sprintf("%03d %02x", ch.Value, ch.Value)
			nameW := font.MeasureString(stripFont, ch.Name).Ceil()
			valW := font.MeasureString(stripFont, valStr).Ceil()
			textW := nameW
			if valW > textW {
				textW = valW
			}
			x0 := mid - textW/2 - pad
			x1 := mid + textW/2 + pad

			fillH := m.LCDHeight * ch.Value / 255
			fillY := m.LCDHeight - fillH
			draw.Draw(img, image.Rect(x0, 0, x0+1, m.LCDHeight), &image.Uniform{color.Black}, image.Point{}, draw.Src)
			draw.Draw(img, image.Rect(x0+1, 0, x1-1, fillY), &image.Uniform{color.Black}, image.Point{}, draw.Src)
			draw.Draw(img, image.Rect(x0+1, fillY, x1-1, m.LCDHeight), &image.Uniform{ch.Primary}, image.Point{}, draw.Src)
			draw.Draw(img, image.Rect(x1-1, 0, x1, m.LCDHeight), &image.Uniform{color.Black}, image.Point{}, draw.Src)

			top := img.SubImage(image.Rect(x0, 0, x1, half)).(*image.RGBA)
			streamdeck.DrawOutlinedText(top, stripFont, cc, color.Black, 2, ch.Name)

			bot := img.SubImage(image.Rect(x0, half, x1, m.LCDHeight)).(*image.RGBA)
			streamdeck.DrawOutlinedText(bot, stripFont, cc, color.Black, 2, valStr)

			metrics := stripFont.Metrics()
			textX := mid - valW/2
			baseY := half + (m.LCDHeight-half-metrics.Height.Ceil())/2 + metrics.Ascent.Ceil() + 2

			nibbleIdx := 5
			if hiNibble[ci] {
				nibbleIdx = 4
			}
			ux := textX + nibbleIdx*charW
			for x := ux - 1; x <= ux+charW; x++ {
				img.Set(x, baseY-1, color.Black)
				img.Set(x, baseY, cc)
				img.Set(x, baseY+1, color.Black)
			}
		}
		dev.SetLCDImage(0, 0, m.LCDWidth, m.LCDHeight, img)
	}

	updateKeys := func() {
		sz := m.KeySize
		b := 5
		for i, p := range pages {
			fg := color.RGBA{200, 200, 200, 255}
			bg := color.RGBA{40, 40, 40, 255}
			txt := streamdeck.TextImageWithFaceSized(streamdeck.MonoBoldSmall, sz, bg, fg, p.Label)
			if i == page {
				img := image.NewRGBA(image.Rect(0, 0, sz, sz))
				draw.Draw(img, img.Bounds(), txt, image.Point{}, draw.Src)
				for y := 0; y < sz; y++ {
					for x := 0; x < sz; x++ {
						if x < b || x >= sz-b || y < b || y >= sz-b {
							img.Set(x, y, color.RGBA{255, 255, 255, 255})
						}
					}
				}
				dev.SetKeyImage(i, img)
			} else {
				dev.SetKeyImage(i, txt)
			}
		}
		for i := len(pages); i < m.Keys; i++ {
			dev.ClearKey(i)
		}
	}

	printValues := func() {
		for _, ch := range channels {
			fmt.Printf("%s=%d ", ch.Name, ch.Value)
		}
		fmt.Println()
	}

	updateLCD()
	updateKeys()

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
			if ev.Encoder != nil {
				p := pages[page]
				ci := p.Start + ev.Encoder.Encoder
				if ci < p.End {
					if ev.Encoder.Delta != 0 {
						step := 1
						if hiNibble[ci] {
							step = 16
						}
						channels[ci].Value = clamp(channels[ci].Value + ev.Encoder.Delta*step)
						updateLCD()
						printValues()
					} else if ev.Encoder.Pressed {
						hiNibble[ci] = !hiNibble[ci]
						updateLCD()
					}
				}
			}
			if ev.Key != nil && ev.Key.Pressed {
				if ev.Key.Key >= 0 && ev.Key.Key < len(pages) && ev.Key.Key != page {
					page = ev.Key.Key
					updateLCD()
					updateKeys()
				}
			}
		case <-sig:
			fmt.Println()
			return
		}
	}
}
