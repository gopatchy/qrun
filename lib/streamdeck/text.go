package streamdeck

import (
	"image"
	"image/color"
	"image/draw"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

func TextImage(bg color.Color, fg color.Color, lines ...string) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, keySize, keySize))
	draw.Draw(img, img.Bounds(), &image.Uniform{bg}, image.Point{}, draw.Src)

	face := basicfont.Face7x13
	metrics := face.Metrics()
	lineHeight := metrics.Height.Ceil()

	totalHeight := lineHeight * len(lines)
	startY := (keySize-totalHeight)/2 + metrics.Ascent.Ceil()

	for i, line := range lines {
		width := font.MeasureString(face, line).Ceil()
		x := (keySize - width) / 2
		y := startY + i*lineHeight

		d := &font.Drawer{
			Dst:  img,
			Src:  &image.Uniform{fg},
			Face: face,
			Dot:  fixed.P(x, y),
		}
		d.DrawString(line)
	}

	return img
}

func (d *Device) SetKeyText(key int, bg color.Color, fg color.Color, text string) error {
	lines := strings.Split(text, "\n")
	return d.SetKeyImage(key, TextImage(bg, fg, lines...))
}
