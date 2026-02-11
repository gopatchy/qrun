package xtouch

import (
	"fmt"

	"gitlab.com/gomidi/midi/v2"
	"gitlab.com/gomidi/midi/v2/drivers"
)

type LCDColor uint8

const (
	ColorBlack   LCDColor = 0
	ColorRed     LCDColor = 1
	ColorGreen   LCDColor = 2
	ColorYellow  LCDColor = 3
	ColorBlue    LCDColor = 4
	ColorMagenta LCDColor = 5
	ColorCyan    LCDColor = 6
	ColorWhite   LCDColor = 7
)

type LEDState uint8

const (
	LEDOff   LEDState = 0
	LEDFlash LEDState = 64
	LEDOn    LEDState = 127
)

type Output struct {
	send     func(msg midi.Message) error
	DeviceID uint8
}

func NewOutput(port drivers.Out, deviceID uint8) (*Output, error) {
	send, err := midi.SendTo(port)
	if err != nil {
		return nil, fmt.Errorf("open output port: %w", err)
	}
	return &Output{send: send, DeviceID: deviceID}, nil
}

func (o *Output) SetFader(fader uint8, value uint8) error {
	cc := CCFaderFirst + fader
	if fader == 8 {
		cc = CCFaderMain
	}
	return o.send(midi.ControlChange(0, cc, value))
}

func (o *Output) SetButtonLED(button uint8, state LEDState) error {
	return o.send(midi.NoteOn(0, button, uint8(state)))
}

func (o *Output) SetEncoderRing(encoder uint8, value uint8) error {
	return o.send(midi.ControlChange(0, CCEncoderFirst+encoder, value))
}

func (o *Output) SetMeter(channel uint8, value uint8) error {
	return o.send(midi.ControlChange(0, CCMeterFirst+channel, value))
}

func (o *Output) SetLCD(lcd uint8, color LCDColor, invertUpper bool, invertLower bool, upper string, lower string) error {
	cc := uint8(color)
	if invertUpper {
		cc |= 0x10
	}
	if invertLower {
		cc |= 0x20
	}

	upper = padOrTruncate(upper, 7)
	lower = padOrTruncate(lower, 7)

	data := []byte{0x00, 0x20, 0x32, o.DeviceID, 0x4C, lcd, cc}
	data = append(data, []byte(upper)...)
	data = append(data, []byte(lower)...)
	return o.send(midi.SysEx(data))
}

func padOrTruncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	for len(s) < n {
		s += " "
	}
	return s
}
