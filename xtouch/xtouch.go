package xtouch

import (
	"fmt"
	"strings"

	"gitlab.com/gomidi/midi/v2"
	"gitlab.com/gomidi/midi/v2/drivers"
)

const (
	DeviceIDXTouch   = 0x14
	DeviceIDExtender = 0x15
)

const (
	CCFootController = 4
	CCFootSwitch1    = 64
	CCFootSwitch2    = 67
	CCFaderFirst     = 70
	CCFaderLast      = 77
	CCFaderMain      = 78
	CCEncoderFirst   = 80
	CCEncoderLast    = 87
	CCJogWheel       = 88
	CCMeterFirst     = 90
	CCMeterLast      = 97
)

const (
	NoteButtonFirst     = 0
	NoteButtonLast      = 103
	NoteFaderTouchFirst = 110
	NoteFaderTouchLast  = 117
	NoteFaderTouchMain  = 118
)

type Event interface {
	String() string
}

type ButtonEvent struct {
	Button  uint8
	Pressed bool
}

func (e ButtonEvent) String() string {
	action := "released"
	if e.Pressed {
		action = "pressed"
	}
	return fmt.Sprintf("Button %d %s", e.Button, action)
}

type FaderEvent struct {
	Fader uint8
	Value uint8
}

func (e FaderEvent) String() string {
	label := fmt.Sprintf("Fader %d", e.Fader)
	if e.Fader == 8 {
		label = "Fader main"
	}
	return fmt.Sprintf("%s = %d", label, e.Value)
}

type FaderTouchEvent struct {
	Fader   uint8
	Touched bool
}

func (e FaderTouchEvent) String() string {
	label := fmt.Sprintf("Fader %d", e.Fader)
	if e.Fader == 8 {
		label = "Fader main"
	}
	action := "released"
	if e.Touched {
		action = "touched"
	}
	return fmt.Sprintf("%s %s", label, action)
}

type EncoderAbsoluteEvent struct {
	Encoder uint8
	Value   uint8
}

func (e EncoderAbsoluteEvent) String() string {
	return fmt.Sprintf("Encoder %d = %d", e.Encoder, e.Value)
}

type EncoderRelativeEvent struct {
	Encoder uint8
	Delta   int
}

func (e EncoderRelativeEvent) String() string {
	return fmt.Sprintf("Encoder %d %+d", e.Encoder, e.Delta)
}

type JogWheelEvent struct {
	Clockwise bool
}

func (e JogWheelEvent) String() string {
	if e.Clockwise {
		return "Jog wheel CW"
	}
	return "Jog wheel CCW"
}

type FootControllerEvent struct {
	Value uint8
}

func (e FootControllerEvent) String() string {
	return fmt.Sprintf("Foot controller = %d", e.Value)
}

type FootSwitchEvent struct {
	Switch  uint8
	Pressed bool
}

func (e FootSwitchEvent) String() string {
	action := "released"
	if e.Pressed {
		action = "pressed"
	}
	return fmt.Sprintf("Foot switch %d %s", e.Switch, action)
}

func FindInPort(substr string) (drivers.In, error) {
	lower := strings.ToLower(substr)
	for _, port := range midi.GetInPorts() {
		if strings.Contains(strings.ToLower(port.String()), lower) {
			return port, nil
		}
	}
	return nil, fmt.Errorf("no MIDI input port matching %q", substr)
}

func FindOutPort(substr string) (drivers.Out, error) {
	lower := strings.ToLower(substr)
	for _, port := range midi.GetOutPorts() {
		if strings.Contains(strings.ToLower(port.String()), lower) {
			return port, nil
		}
	}
	return nil, fmt.Errorf("no MIDI output port matching %q", substr)
}

type EncoderMode int

const (
	EncoderAbsolute EncoderMode = iota
	EncoderRelative
)

type Decoder struct {
	EncoderMode EncoderMode
}

func (d *Decoder) Decode(msg midi.Message) Event {
	switch {
	case msg.Is(midi.NoteOnMsg):
		var channel, key, velocity uint8
		msg.GetNoteOn(&channel, &key, &velocity)
		return decodeNoteOn(key, velocity)

	case msg.Is(midi.NoteOffMsg):
		var channel, key, velocity uint8
		msg.GetNoteOff(&channel, &key, &velocity)
		return decodeNoteOn(key, 0)

	case msg.Is(midi.ControlChangeMsg):
		var channel, controller, value uint8
		msg.GetControlChange(&channel, &controller, &value)
		return d.decodeCC(controller, value)
	}
	return nil
}

func decodeNoteOn(key, velocity uint8) Event {
	pressed := velocity > 0

	if key >= NoteButtonFirst && key <= NoteButtonLast {
		return ButtonEvent{Button: key, Pressed: pressed}
	}
	if key >= NoteFaderTouchFirst && key <= NoteFaderTouchLast {
		return FaderTouchEvent{Fader: key - NoteFaderTouchFirst, Touched: pressed}
	}
	if key == NoteFaderTouchMain {
		return FaderTouchEvent{Fader: 8, Touched: pressed}
	}
	return nil
}

func (d *Decoder) decodeCC(controller, value uint8) Event {
	switch {
	case controller >= CCFaderFirst && controller <= CCFaderLast:
		return FaderEvent{Fader: controller - CCFaderFirst, Value: value}
	case controller == CCFaderMain:
		return FaderEvent{Fader: 8, Value: value}
	case controller >= CCEncoderFirst && controller <= CCEncoderLast:
		enc := controller - CCEncoderFirst
		if d.EncoderMode == EncoderRelative {
			delta := 0
			switch value {
			case 65:
				delta = 1
			case 1:
				delta = -1
			}
			return EncoderRelativeEvent{Encoder: enc, Delta: delta}
		}
		return EncoderAbsoluteEvent{Encoder: enc, Value: value}
	case controller == CCJogWheel:
		return JogWheelEvent{Clockwise: value == 65}
	case controller == CCFootController:
		return FootControllerEvent{Value: value}
	case controller == CCFootSwitch1:
		return FootSwitchEvent{Switch: 1, Pressed: value > 0}
	case controller == CCFootSwitch2:
		return FootSwitchEvent{Switch: 2, Pressed: value > 0}
	}
	return nil
}
