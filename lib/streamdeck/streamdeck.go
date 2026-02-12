package streamdeck

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"time"

	xdraw "golang.org/x/image/draw"

	"rafaelmartins.com/p/usbhid"
)

const elgatoVendorID = 0x0fd9

type Model struct {
	Name      string
	Keys      int
	KeyRows   int
	KeyCols   int
	KeySize   int
	FlipKeys  bool
	Encoders  int
	LCDWidth  int
	LCDHeight int
}

var ModelXL = Model{
	Name:     "XL",
	Keys:     32,
	KeyRows:  4,
	KeyCols:  8,
	KeySize:  96,
	FlipKeys: true,
}

var ModelPlus = Model{
	Name:     "Plus",
	Keys:     8,
	KeyRows:  2,
	KeyCols:  4,
	KeySize:  120,
	Encoders: 4,
	LCDWidth: 800,
	LCDHeight: 100,
}

var productModels = map[uint16]*Model{
	0x006c: &ModelXL,
	0x008f: &ModelXL,
	0x0084: &ModelPlus,
}

type Device struct {
	dev   *usbhid.Device
	model *Model
}

func Open() (*Device, error) {
	devices, err := usbhid.Enumerate(func(dev *usbhid.Device) bool {
		return dev.VendorId() == elgatoVendorID && productModels[dev.ProductId()] != nil
	})
	if err != nil {
		return nil, fmt.Errorf("streamdeck: enumerate: %w", err)
	}
	if len(devices) == 0 {
		return nil, fmt.Errorf("streamdeck: no device found")
	}

	dev := devices[0]
	model := productModels[dev.ProductId()]
	if err := dev.Open(true); err != nil {
		return nil, fmt.Errorf("streamdeck: open: %w", err)
	}

	return &Device{dev: dev, model: model}, nil
}

func OpenModel(m *Model) (*Device, error) {
	devices, err := usbhid.Enumerate(func(dev *usbhid.Device) bool {
		if dev.VendorId() != elgatoVendorID {
			return false
		}
		return productModels[dev.ProductId()] == m
	})
	if err != nil {
		return nil, fmt.Errorf("streamdeck: enumerate: %w", err)
	}
	if len(devices) == 0 {
		return nil, fmt.Errorf("streamdeck: no %s device found", m.Name)
	}

	dev := devices[0]
	if err := dev.Open(true); err != nil {
		return nil, fmt.Errorf("streamdeck: open: %w", err)
	}

	return &Device{dev: dev, model: m}, nil
}

func (d *Device) Model() *Model     { return d.model }
func (d *Device) Close() error      { return d.dev.Close() }
func (d *Device) SerialNumber() string { return d.dev.SerialNumber() }
func (d *Device) Product() string    { return d.dev.Product() }

func (d *Device) FirmwareVersion() (string, error) {
	buf, err := d.dev.GetFeatureReport(5)
	if err != nil {
		return "", err
	}
	b, _, _ := bytes.Cut(buf[5:], []byte{0})
	return string(b), nil
}

func (d *Device) SetBrightness(perc byte) error {
	if perc > 100 {
		perc = 100
	}
	pl := make([]byte, d.dev.GetFeatureReportLength())
	pl[0] = 0x08
	pl[1] = perc
	return d.dev.SetFeatureReport(3, pl)
}

func (d *Device) Reset() error {
	pl := make([]byte, d.dev.GetFeatureReportLength())
	pl[0] = 0x02
	return d.dev.SetFeatureReport(3, pl)
}

func (d *Device) SetKeyColor(key int, c color.Color) error {
	sz := d.model.KeySize
	img := image.NewRGBA(image.Rect(0, 0, sz, sz))
	xdraw.Draw(img, img.Bounds(), &image.Uniform{c}, image.Point{}, xdraw.Src)
	return d.SetKeyImage(key, img)
}

func (d *Device) SetKeyImage(key int, img image.Image) error {
	if key < 0 || key >= d.model.Keys {
		return fmt.Errorf("streamdeck: invalid key %d", key)
	}

	sz := d.model.KeySize
	scaled := image.NewRGBA(image.Rect(0, 0, sz, sz))
	xdraw.BiLinear.Scale(scaled, scaled.Bounds(), img, img.Bounds(), xdraw.Over, nil)

	var src image.Image = scaled
	if d.model.FlipKeys {
		flipped := image.NewRGBA(scaled.Bounds())
		for y := 0; y < sz; y++ {
			for x := 0; x < sz; x++ {
				flipped.Set(sz-1-x, sz-1-y, scaled.At(x, y))
			}
		}
		src = flipped
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, src, &jpeg.Options{Quality: 100}); err != nil {
		return err
	}

	return d.sendKeyImage(byte(key), buf.Bytes())
}

func (d *Device) ClearKey(key int) error {
	return d.SetKeyColor(key, color.Black)
}

func (d *Device) ClearAllKeys() error {
	for i := 0; i < d.model.Keys; i++ {
		if err := d.ClearKey(i); err != nil {
			return err
		}
	}
	return nil
}

func (d *Device) sendKeyImage(key byte, imgData []byte) error {
	reportLen := d.dev.GetOutputReportLength()
	hdrLen := uint16(8)
	payloadLen := reportLen - hdrLen

	var page uint16
	for start := uint16(0); start < uint16(len(imgData)); page++ {
		end := start + payloadLen
		last := byte(0)
		if end >= uint16(len(imgData)) {
			end = uint16(len(imgData))
			last = 1
		}

		chunk := imgData[start:end]
		hdr := []byte{
			0x02,
			0x07,
			key,
			last,
			byte(len(chunk)),
			byte(len(chunk) >> 8),
			byte(page),
			byte(page >> 8),
		}

		payload := append(hdr, chunk...)
		padding := make([]byte, reportLen-uint16(len(payload)))
		payload = append(payload, padding...)

		if err := d.dev.SetOutputReport(2, payload); err != nil {
			return err
		}
		start = end
	}
	return nil
}

func (d *Device) SetLCDImage(x, y, w, h int, img image.Image) error {
	if d.model.LCDWidth == 0 {
		return fmt.Errorf("streamdeck: %s has no LCD", d.model.Name)
	}

	scaled := image.NewRGBA(image.Rect(0, 0, w, h))
	xdraw.BiLinear.Scale(scaled, scaled.Bounds(), img, img.Bounds(), xdraw.Over, nil)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, scaled, &jpeg.Options{Quality: 100}); err != nil {
		return err
	}

	return d.sendLCDImage(uint16(x), uint16(y), uint16(w), uint16(h), buf.Bytes())
}

func (d *Device) SetLCDColor(x, y, w, h int, c color.Color) error {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	xdraw.Draw(img, img.Bounds(), &image.Uniform{c}, image.Point{}, xdraw.Src)
	return d.SetLCDImage(x, y, w, h, img)
}

func (d *Device) sendLCDImage(x, y, w, h uint16, imgData []byte) error {
	reportLen := d.dev.GetOutputReportLength()
	hdrLen := uint16(16)
	payloadLen := reportLen - hdrLen

	var page uint16
	for start := uint16(0); start < uint16(len(imgData)); page++ {
		end := start + payloadLen
		last := byte(0)
		if end >= uint16(len(imgData)) {
			end = uint16(len(imgData))
			last = 1
		}

		chunk := imgData[start:end]
		hdr := make([]byte, 16)
		hdr[0] = 0x02
		hdr[1] = 0x0C
		binary.LittleEndian.PutUint16(hdr[2:], x)
		binary.LittleEndian.PutUint16(hdr[4:], y)
		binary.LittleEndian.PutUint16(hdr[6:], w)
		binary.LittleEndian.PutUint16(hdr[8:], h)
		hdr[10] = last
		binary.LittleEndian.PutUint16(hdr[11:], page)
		binary.LittleEndian.PutUint16(hdr[13:], uint16(len(chunk)))
		hdr[15] = 0

		payload := append(hdr, chunk...)
		padding := make([]byte, reportLen-uint16(len(payload)))
		payload = append(payload, padding...)

		if err := d.dev.SetOutputReport(2, payload); err != nil {
			return err
		}
		start = end
	}
	return nil
}

type KeyEvent struct {
	Key     int
	Pressed bool
	Time    time.Time
}

type EncoderEvent struct {
	Encoder int
	Pressed bool
	Delta   int
	Time    time.Time
}

type TouchEvent struct {
	X    int
	Y    int
	X2   int
	Y2   int
	Type TouchType
	Time time.Time
}

type TouchType int

const (
	TouchShort TouchType = 1
	TouchLong  TouchType = 2
	TouchSwipe TouchType = 3
)

type InputEvent struct {
	Key     *KeyEvent
	Encoder *EncoderEvent
	Touch   *TouchEvent
}

func (d *Device) ReadInput(ch chan<- InputEvent) error {
	keyStates := make([]byte, d.model.Keys)
	encoderStates := make([]byte, d.model.Encoders)
	for {
		_, buf, err := d.dev.GetInputReport()
		if err != nil {
			return err
		}
		if len(buf) < 4 {
			continue
		}

		t := time.Now()
		switch buf[0] {
		case 0x00:
			keyStart := 3
			for i := 0; i < d.model.Keys; i++ {
				if keyStart+i >= len(buf) {
					break
				}
				st := buf[keyStart+i]
				if st != keyStates[i] {
					ch <- InputEvent{Key: &KeyEvent{
						Key:     i,
						Pressed: st > 0,
						Time:    t,
					}}
					keyStates[i] = st
				}
			}
		case 0x03:
			if d.model.Encoders == 0 || len(buf) < 8 {
				continue
			}
			subType := buf[3]
			switch subType {
			case 0x00:
				for i := 0; i < d.model.Encoders; i++ {
					st := buf[4+i]
					if st != encoderStates[i] {
						ch <- InputEvent{Encoder: &EncoderEvent{
							Encoder: i,
							Pressed: st > 0,
							Time:    t,
						}}
						encoderStates[i] = st
					}
				}
			case 0x01:
				for i := 0; i < d.model.Encoders; i++ {
					delta := int(int8(buf[4+i]))
					if delta != 0 {
						ch <- InputEvent{Encoder: &EncoderEvent{
							Encoder: i,
							Delta:   delta,
							Time:    t,
						}}
					}
				}
			}
		case 0x02:
			if len(buf) < 14 {
				continue
			}
			subType := TouchType(buf[3])
			x := int(binary.LittleEndian.Uint16(buf[5:7]))
			y := int(binary.LittleEndian.Uint16(buf[7:9]))
			ev := TouchEvent{
				X:    x,
				Y:    y,
				Type: subType,
				Time: t,
			}
			if subType == TouchSwipe {
				ev.X2 = int(binary.LittleEndian.Uint16(buf[9:11]))
				ev.Y2 = int(binary.LittleEndian.Uint16(buf[11:13]))
			}
			ch <- InputEvent{Touch: &ev}
		}
	}
}

func (d *Device) ReadKeys(ch chan<- KeyEvent) error {
	input := make(chan InputEvent, 64)
	go func() {
		for ev := range input {
			if ev.Key != nil {
				ch <- *ev.Key
			}
		}
	}()
	return d.ReadInput(input)
}
