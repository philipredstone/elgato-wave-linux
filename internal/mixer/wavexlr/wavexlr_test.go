package wavexlr

import (
	"bytes"
	"testing"

	"github.com/philipredstone/elgato-wave-linux/internal/mixer"
)

type fakeDevice struct {
	config []byte
	writes int
}

func (f *fakeDevice) ControlIn(_ uint8, value, _ uint16, length int) ([]byte, error) {
	if value == valConfig {
		return bytes.Clone(f.config), nil
	}
	return make([]byte, length), nil
}

func (f *fakeDevice) ControlOut(_ uint8, _, _ uint16, data []byte) error {
	f.config = bytes.Clone(data)
	f.writes++
	return nil
}

func TestReadDecodesConfigBlock(t *testing.T) {
	dev := &fakeDevice{config: make([]byte, configLen)}
	dev.config[0], dev.config[1] = 0x00, 0x4b // 75.0 dB
	dev.config[4] = 1
	dev.config[9], dev.config[10] = 0x00, 0xf4 // -12.0 dB
	dev.config[33] = 1

	s, err := New(dev).Read()
	if err != nil {
		t.Fatal(err)
	}
	want := mixer.Settings{"gain": 0x4b00, "mute": 1, "hp": 0xf400, "knob": 0, "low-z": 1}
	for k, v := range want {
		if s[k] != v {
			t.Errorf("%s = %#x, want %#x", k, s[k], v)
		}
	}
}

func TestWritePreservesUnknownBytes(t *testing.T) {
	dev := &fakeDevice{config: bytes.Repeat([]byte{0xaa}, configLen)}
	if err := New(dev).Write(mixer.Settings{"gain": 0x3840}); err != nil {
		t.Fatal(err)
	}
	if dev.config[0] != 0x40 || dev.config[1] != 0x38 {
		t.Errorf("gain bytes = % x, want 40 38", dev.config[:2])
	}
	for i := 2; i < configLen; i++ {
		if dev.config[i] != 0xaa {
			t.Fatalf("byte %d changed to %#x", i, dev.config[i])
		}
	}
}

func TestWriteRejectsUnknownField(t *testing.T) {
	dev := &fakeDevice{config: make([]byte, configLen)}
	if err := New(dev).Write(mixer.Settings{"phantom": 1}); err == nil {
		t.Fatal("want error")
	}
	if dev.writes != 0 {
		t.Error("wrote to device despite error")
	}
}
