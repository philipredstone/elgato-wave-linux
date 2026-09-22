package device

import (
	"github.com/philipredstone/elgato-wave-linux/internal/mixer"
	"github.com/philipredstone/elgato-wave-linux/internal/mixer/wavexlr"
	"github.com/philipredstone/elgato-wave-linux/internal/usb"
)

type Model struct {
	Name     string
	ID       usb.ID
	NewMixer func(*usb.Device) mixer.Mixer // nil = protocol unknown
	Tested   bool
}

// keep udev/60-waved.rules in sync
var Models = []Model{
	{
		Name:     "Elgato Wave XLR",
		ID:       usb.ID{Vendor: "0fd9", Product: "007d"},
		NewMixer: func(d *usb.Device) mixer.Mixer { return wavexlr.New(d) },
		Tested:   true,
	},
	{
		Name: "Elgato Wave:3", // never tried
		ID:   usb.ID{Vendor: "0fd9", Product: "0070"},
	},
}

type Device struct {
	*usb.Device
	Model Model
	Mixer mixer.Mixer
}

type Selector struct {
	models []Model
}

func Any() Selector { return Selector{models: Models} }

func ByID(id usb.ID) Selector {
	for _, m := range Models {
		if m.ID == id {
			return Selector{models: []Model{m}}
		}
	}
	return Selector{models: []Model{{Name: "USB audio device " + id.String(), ID: id}}}
}

func (s Selector) Find() (*Device, bool) {
	ids := make([]usb.ID, len(s.models))
	for i, m := range s.models {
		ids[i] = m.ID
	}
	u, ok := usb.Find(ids...)
	if !ok {
		return nil, false
	}
	for _, m := range s.models {
		if m.ID != u.ID {
			continue
		}
		d := &Device{Device: u, Model: m}
		if m.NewMixer != nil {
			d.Mixer = m.NewMixer(u)
		}
		return d, true
	}
	return nil, false
}

func (d *Device) StateKey() string {
	key := d.ID.Vendor + "-" + d.ID.Product
	if d.Serial != "" {
		key += "-" + d.Serial
	}
	return key
}
