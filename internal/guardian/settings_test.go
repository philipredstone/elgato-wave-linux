package guardian

import (
	"io"
	"log/slog"
	"maps"
	"testing"

	"github.com/philipredstone/elgato-wave-linux/internal/device"
	"github.com/philipredstone/elgato-wave-linux/internal/mixer"
	"github.com/philipredstone/elgato-wave-linux/internal/state"
	"github.com/philipredstone/elgato-wave-linux/internal/usb"
)

type fakeMixer struct{ values mixer.Settings }

func (f *fakeMixer) Fields() []mixer.Field {
	return []mixer.Field{
		{Name: "gain", Persist: true, Format: mixer.FormatQ8},
		{Name: "knob", Format: mixer.FormatBool},
	}
}
func (f *fakeMixer) Read() (mixer.Settings, error)   { return maps.Clone(f.values), nil }
func (f *fakeMixer) Write(s mixer.Settings) error    { maps.Copy(f.values, s); return nil }
func (f *fakeMixer) Meters() (uint32, uint32, error) { return 0, 0, nil }
func (f *fakeMixer) Muted() (bool, error)            { return false, nil }
func (f *fakeMixer) Info() (mixer.Info, error)       { return mixer.Info{}, nil }

func newTestDevice(node string, mx *fakeMixer) *device.Device {
	return &device.Device{
		Device: &usb.Device{ID: usb.ID{Vendor: "0fd9", Product: "007d"}, Node: node, Serial: "S1"},
		Mixer:  mx,
	}
}

func newTestSync(t *testing.T) *settingsSync {
	return &settingsSync{
		store: state.Store{Dir: t.TempDir()},
		log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func TestRestoreAfterPowerLoss(t *testing.T) {
	s := newTestSync(t)
	mx := &fakeMixer{values: mixer.Settings{"gain": 14400, "knob": 1}}
	s.restore(newTestDevice("/dev/bus/usb/007/005", mx))

	// Replug: firmware defaults on a new enumeration.
	mx.values["gain"] = 19200
	mx.values["knob"] = 2
	s.restore(newTestDevice("/dev/bus/usb/007/006", mx))

	if mx.values["gain"] != 14400 {
		t.Errorf("gain = %d, want restored 14400", mx.values["gain"])
	}
	if mx.values["knob"] != 2 {
		t.Error("restore touched a field that is not persisted")
	}
}

func TestRestoreIsOncePerEnumeration(t *testing.T) {
	s := newTestSync(t)
	mx := &fakeMixer{values: mixer.Settings{"gain": 14400}}
	dev := newTestDevice("/dev/bus/usb/007/005", mx)
	s.restore(dev)

	mx.values["gain"] = 5000 // user turns the knob
	s.restore(dev)
	if mx.values["gain"] != 5000 {
		t.Error("restore overwrote a live change on the same enumeration")
	}
}

func TestChangeNeedsConfirmation(t *testing.T) {
	s := newTestSync(t)
	mx := &fakeMixer{values: mixer.Settings{"gain": 14400}}
	s.restore(newTestDevice("/dev/bus/usb/007/005", mx))

	mx.values["gain"] = 0 // glitch while losing power
	s.record(mx)
	if s.saved["gain"] != 14400 {
		t.Fatal("saved an unconfirmed change")
	}
	mx.values["gain"] = 14400
	s.record(mx)

	mx.values["gain"] = 9000
	s.record(mx)
	s.record(mx)
	if s.saved["gain"] != 9000 {
		t.Errorf("saved gain = %d, want confirmed 9000", s.saved["gain"])
	}
}
