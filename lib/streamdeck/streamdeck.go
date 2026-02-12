package streamdeck

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"time"

	xdraw "golang.org/x/image/draw"

	"rafaelmartins.com/p/usbhid"
)

const (
	elgatoVendorID = 0x0fd9
	keyCount       = 32
	keyRows        = 4
	keyCols        = 8
	keySize        = 96
	keyStart       = 3
)

var xlProductIDs = map[uint16]bool{
	0x006c: true,
	0x008f: true,
}

type Device struct {
	dev *usbhid.Device
}

func Open() (*Device, error) {
	devices, err := usbhid.Enumerate(func(dev *usbhid.Device) bool {
		return dev.VendorId() == elgatoVendorID && xlProductIDs[dev.ProductId()]
	})
	if err != nil {
		return nil, fmt.Errorf("streamdeck: enumerate: %w", err)
	}
	if len(devices) == 0 {
		return nil, fmt.Errorf("streamdeck: no XL device found")
	}

	dev := devices[0]
	if err := dev.Open(true); err != nil {
		return nil, fmt.Errorf("streamdeck: open: %w", err)
	}

	return &Device{dev: dev}, nil
}

func (d *Device) Close() error {
	return d.dev.Close()
}

func (d *Device) SerialNumber() string {
	return d.dev.SerialNumber()
}

func (d *Device) Product() string {
	return d.dev.Product()
}

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
	img := image.NewRGBA(image.Rect(0, 0, keySize, keySize))
	xdraw.Draw(img, img.Bounds(), &image.Uniform{c}, image.Point{}, xdraw.Src)
	return d.SetKeyImage(key, img)
}

func (d *Device) SetKeyImage(key int, img image.Image) error {
	if key < 0 || key >= keyCount {
		return fmt.Errorf("streamdeck: invalid key %d", key)
	}

	scaled := image.NewRGBA(image.Rect(0, 0, keySize, keySize))
	xdraw.BiLinear.Scale(scaled, scaled.Bounds(), img, img.Bounds(), xdraw.Over, nil)

	flipped := image.NewRGBA(scaled.Bounds())
	for y := 0; y < keySize; y++ {
		for x := 0; x < keySize; x++ {
			flipped.Set(keySize-1-x, keySize-1-y, scaled.At(x, y))
		}
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, flipped, &jpeg.Options{Quality: 100}); err != nil {
		return err
	}

	return d.sendImage(byte(key), buf.Bytes())
}

func (d *Device) ClearKey(key int) error {
	return d.SetKeyColor(key, color.Black)
}

func (d *Device) ClearAllKeys() error {
	for i := 0; i < keyCount; i++ {
		if err := d.ClearKey(i); err != nil {
			return err
		}
	}
	return nil
}

func (d *Device) sendImage(key byte, imgData []byte) error {
	reportLen := d.dev.GetOutputReportLength()
	hdrLen := uint16(7)
	payloadLen := reportLen - hdrLen

	var page byte
	for start := uint16(0); start < uint16(len(imgData)); page++ {
		end := start + payloadLen
		last := byte(0)
		if end >= uint16(len(imgData)) {
			end = uint16(len(imgData))
			last = 1
		}

		chunk := imgData[start:end]
		hdr := []byte{
			0x07,
			key,
			last,
			byte(len(chunk)),
			byte(len(chunk) >> 8),
			page,
			0,
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

type KeyEvent struct {
	Key     int
	Pressed bool
	Time    time.Time
}

func (d *Device) ReadKeys(ch chan<- KeyEvent) error {
	states := make([]byte, keyCount)
	for {
		_, buf, err := d.dev.GetInputReport()
		if err != nil {
			return err
		}

		if int(keyStart+keyCount) > len(buf) {
			continue
		}

		t := time.Now()
		for i := 0; i < keyCount; i++ {
			st := buf[keyStart+i]
			if st != states[i] {
				ch <- KeyEvent{
					Key:     i,
					Pressed: st > 0,
					Time:    t,
				}
				states[i] = st
			}
		}
	}
}

func KeyCount() int { return keyCount }
func KeyRows() int  { return keyRows }
func KeyCols() int  { return keyCols }
