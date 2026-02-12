package streamdeck

import (
	"embed"
	"image"
	"image/color"
	"image/draw"
	"log"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

//go:embed fonts/*.ttf
var fontFS embed.FS

var (
	MonoRegular font.Face
	MonoMedium  font.Face
	MonoBold    font.Face
	Regular     font.Face
	Bold        font.Face
)

func init() {
	MonoRegular = loadFace("fonts/AtkinsonHyperlegibleMono-Regular.ttf", 72)
	MonoMedium = loadFace("fonts/AtkinsonHyperlegibleMono-Medium.ttf", 72)
	MonoBold = loadFace("fonts/AtkinsonHyperlegibleMono-Bold.ttf", 72)
	Regular = loadFace("fonts/AtkinsonHyperlegible-Regular.ttf", 16)
	Bold = loadFace("fonts/AtkinsonHyperlegible-Bold.ttf", 16)
}

func loadFace(path string, size float64) font.Face {
	data, err := fontFS.ReadFile(path)
	if err != nil {
		log.Fatalf("streamdeck: read font %s: %v", path, err)
	}
	f, err := opentype.Parse(data)
	if err != nil {
		log.Fatalf("streamdeck: parse font %s: %v", path, err)
	}
	face, err := opentype.NewFace(f, &opentype.FaceOptions{
		Size:    size,
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		log.Fatalf("streamdeck: create face %s: %v", path, err)
	}
	return face
}

func DrawText(img *image.RGBA, face font.Face, fg color.Color, lines ...string) {
	metrics := face.Metrics()
	lineHeight := metrics.Height.Ceil()
	bounds := img.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()

	totalHeight := lineHeight * len(lines)
	startY := (h-totalHeight)/2 + metrics.Ascent.Ceil()

	for i, line := range lines {
		width := font.MeasureString(face, line).Ceil()
		x := (w - width) / 2
		y := startY + i*lineHeight

		d := &font.Drawer{
			Dst:  img,
			Src:  &image.Uniform{fg},
			Face: face,
			Dot:  fixed.P(bounds.Min.X+x, bounds.Min.Y+y),
		}
		d.DrawString(line)
	}
}

func TextImageSized(size int, bg color.Color, fg color.Color, lines ...string) image.Image {
	return TextImageWithFaceSized(MonoMedium, size, bg, fg, lines...)
}

func TextImageWithFaceSized(face font.Face, size int, bg color.Color, fg color.Color, lines ...string) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.Draw(img, img.Bounds(), &image.Uniform{bg}, image.Point{}, draw.Src)
	DrawText(img, face, fg, lines...)
	return img
}

func (d *Device) SetKeyText(key int, bg color.Color, fg color.Color, text string) error {
	lines := strings.Split(text, "\n")
	return d.SetKeyImage(key, TextImageSized(d.model.KeySize, bg, fg, lines...))
}

func (d *Device) SetKeyBoldText(key int, bg color.Color, fg color.Color, text string) error {
	lines := strings.Split(text, "\n")
	return d.SetKeyImage(key, TextImageWithFaceSized(MonoBold, d.model.KeySize, bg, fg, lines...))
}
