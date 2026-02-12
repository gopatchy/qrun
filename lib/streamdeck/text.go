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
	MonoRegular      font.Face
	MonoMedium       font.Face
	MonoBold         font.Face
	MonoRegularSmall font.Face
	MonoMediumSmall  font.Face
	MonoBoldSmall    font.Face
	Regular          font.Face
	Bold             font.Face
)

func init() {
	MonoRegular = LoadFace("fonts/AtkinsonHyperlegibleMono-Regular.ttf", 72)
	MonoMedium = LoadFace("fonts/AtkinsonHyperlegibleMono-Medium.ttf", 72)
	MonoBold = LoadFace("fonts/AtkinsonHyperlegibleMono-Bold.ttf", 72)
	MonoRegularSmall = LoadFace("fonts/AtkinsonHyperlegibleMono-Regular.ttf", 40)
	MonoMediumSmall = LoadFace("fonts/AtkinsonHyperlegibleMono-Medium.ttf", 40)
	MonoBoldSmall = LoadFace("fonts/AtkinsonHyperlegibleMono-Bold.ttf", 40)
	Regular = LoadFace("fonts/AtkinsonHyperlegible-Regular.ttf", 16)
	Bold = LoadFace("fonts/AtkinsonHyperlegible-Bold.ttf", 16)
}

func LoadFace(path string, size float64) font.Face {
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

func DrawOutlinedText(img *image.RGBA, face font.Face, fg color.Color, outline color.Color, thickness int, lines ...string) {
	for dx := -thickness; dx <= thickness; dx++ {
		for dy := -thickness; dy <= thickness; dy++ {
			if dx == 0 && dy == 0 {
				continue
			}
			drawTextOffset(img, face, outline, dx, dy, lines...)
		}
	}
	drawTextOffset(img, face, fg, 0, 0, lines...)
}

func DrawText(img *image.RGBA, face font.Face, fg color.Color, lines ...string) {
	drawTextOffset(img, face, fg, 0, 0, lines...)
}

func drawTextOffset(img *image.RGBA, face font.Face, fg color.Color, dx, dy int, lines ...string) {
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
			Dot:  fixed.P(bounds.Min.X+x+dx, bounds.Min.Y+y+dy),
		}
		d.DrawString(line)
	}
}

type TextSpan struct {
	Text    string
	Color   color.Color
	Outline color.Color
}

func DrawOutlinedSpans(img *image.RGBA, face font.Face, defaultOutline color.Color, thickness int, lines ...[]TextSpan) {
	var full []string
	for _, line := range lines {
		s := ""
		for _, span := range line {
			s += span.Text
		}
		full = append(full, s)
	}

	metrics := face.Metrics()
	lineHeight := metrics.Height.Ceil()
	bounds := img.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()
	totalHeight := lineHeight * len(lines)
	startY := (h-totalHeight)/2 + metrics.Ascent.Ceil()

	for dx := -thickness; dx <= thickness; dx++ {
		for dy := -thickness; dy <= thickness; dy++ {
			if dx == 0 && dy == 0 {
				continue
			}
			for i, line := range lines {
				width := font.MeasureString(face, full[i]).Ceil()
				x := (w - width) / 2
				y := startY + i*lineHeight
				d := &font.Drawer{
					Dst:  img,
					Face: face,
					Dot:  fixed.P(bounds.Min.X+x+dx, bounds.Min.Y+y+dy),
				}
				for _, span := range line {
					ol := defaultOutline
					if span.Outline != nil {
						ol = span.Outline
					}
					d.Src = &image.Uniform{ol}
					d.DrawString(span.Text)
				}
			}
		}
	}

	for i, line := range lines {
		width := font.MeasureString(face, full[i]).Ceil()
		x := (w - width) / 2
		y := startY + i*lineHeight
		d := &font.Drawer{
			Dst:  img,
			Face: face,
			Dot:  fixed.P(bounds.Min.X+x, bounds.Min.Y+y),
		}
		for _, span := range line {
			d.Src = &image.Uniform{span.Color}
			d.DrawString(span.Text)
		}
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

func (d *Device) SetKeyTextWithFace(key int, face font.Face, bg color.Color, fg color.Color, text string) error {
	lines := strings.Split(text, "\n")
	return d.SetKeyImage(key, TextImageWithFaceSized(face, d.model.KeySize, bg, fg, lines...))
}

func (d *Device) SetKeyBoldText(key int, bg color.Color, fg color.Color, text string) error {
	lines := strings.Split(text, "\n")
	return d.SetKeyImage(key, TextImageWithFaceSized(MonoBold, d.model.KeySize, bg, fg, lines...))
}
