// Package wavexlr: the control protocol, ported from openwave. NOTES.md
// has the details.
package wavexlr

import (
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/philipredstone/elgato-wave-linux/internal/mixer"
)

type Transport interface {
	ControlIn(request uint8, value, index uint16, length int) ([]byte, error)
	ControlOut(request uint8, value, index uint16, data []byte) error
}

const (
	reqGet = 0x85
	reqSet = 0x05
	index  = 0x3303 // high byte for the firmware, low byte keeps snd-usb-audio bound

	valConfig  = 0x0000
	valMeters  = 0x0001
	valDevInfo = 0x000a

	configLen  = 34
	metersLen  = 10
	devInfoLen = 51

	MaxGainDB = 75
)

type layout struct{ offset, size int }

// LE, dB is Q8.8
var layouts = map[string]layout{
	"gain":  {0, 2}, // 0..75 dB
	"mute":  {4, 1},
	"hp":    {9, 2},  // -128..0 dB
	"knob":  {14, 1}, // 1 = gain, 2 = hp volume
	"low-z": {33, 1},
}

var fields = []mixer.Field{
	{Name: "gain", Usage: fmt.Sprintf("dB, 0 to %d", MaxGainDB), Persist: true,
		Format: mixer.FormatQ8, Parse: mixer.ParseQ8(0, MaxGainDB)},
	{Name: "mute", Usage: "on|off", Persist: true,
		Format: mixer.FormatBool, Parse: mixer.ParseBool},
	{Name: "hp", Usage: "dB, -128 to 0", Persist: true,
		Format: mixer.FormatQ8, Parse: mixer.ParseQ8(-128, 0)},
	{Name: "low-z", Usage: "on|off", Persist: true,
		Format: mixer.FormatBool, Parse: mixer.ParseBool},
	{Name: "knob", Format: formatKnob},
}

func formatKnob(raw int) string {
	if raw == 2 {
		return "headphone volume"
	}
	return "gain"
}

type Mixer struct{ t Transport }

func New(t Transport) *Mixer { return &Mixer{t: t} }

func (*Mixer) Fields() []mixer.Field { return fields }

func (m *Mixer) Read() (mixer.Settings, error) {
	block, err := m.readConfig()
	if err != nil {
		return nil, err
	}
	s := make(mixer.Settings, len(layouts))
	for name, l := range layouts {
		s[name] = get(block, l)
	}
	return s, nil
}

func (m *Mixer) Write(s mixer.Settings) error {
	block, err := m.readConfig()
	if err != nil {
		return err
	}
	for name, v := range s {
		l, ok := layouts[name]
		if !ok {
			return fmt.Errorf("unknown setting %q", name)
		}
		put(block, l, v)
	}
	return m.t.ControlOut(reqSet, valConfig, index, block)
}

func (m *Mixer) Muted() (bool, error) {
	block, err := m.readConfig()
	if err != nil {
		return false, err
	}
	return get(block, layouts["mute"]) != 0, nil
}

func (m *Mixer) Meters() (left, right uint32, err error) {
	b, err := m.read(valMeters, metersLen)
	if err != nil {
		return 0, 0, err
	}
	return binary.LittleEndian.Uint32(b[0:]), binary.LittleEndian.Uint32(b[4:]), nil
}

func (m *Mixer) Info() (mixer.Info, error) {
	b, err := m.read(valDevInfo, devInfoLen)
	if err != nil {
		return mixer.Info{}, err
	}
	return mixer.Info{
		Firmware: fmt.Sprintf("%d.%d.%d", b[6], b[7], b[8]),
		Serial:   strings.TrimRight(string(b[27:47]), "\x00"),
	}, nil
}

func (m *Mixer) readConfig() ([]byte, error) { return m.read(valConfig, configLen) }

func (m *Mixer) read(value uint16, length int) ([]byte, error) {
	b, err := m.t.ControlIn(reqGet, value, index, length)
	if err != nil {
		return nil, err
	}
	if len(b) < length {
		return nil, fmt.Errorf("short read of %#04x: %d of %d bytes", value, len(b), length)
	}
	return b, nil
}

func get(block []byte, l layout) int {
	if l.size == 2 {
		return int(binary.LittleEndian.Uint16(block[l.offset:]))
	}
	return int(block[l.offset])
}

func put(block []byte, l layout, v int) {
	if l.size == 2 {
		binary.LittleEndian.PutUint16(block[l.offset:], uint16(v))
	} else {
		block[l.offset] = byte(v)
	}
}
