package guardian

import (
	"time"

	"github.com/philipredstone/elgato-wave-linux/internal/device"
)

// zeros can mean muted, nothing plugged in, or wedged. the meters still
// work when wedged, so they tell the difference.
type silenceVerdict int

const (
	silenceUnknown silenceVerdict = iota
	silenceExpected
	silenceGenuine
	silenceWedged
)

const meterSamples = 3

func checkSilence(dev *device.Device) (silenceVerdict, error) {
	mx := dev.Mixer
	if mx == nil {
		return silenceUnknown, errNoMixer
	}
	muted, err := mx.Muted()
	if err != nil {
		return silenceUnknown, err
	}
	if muted {
		return silenceExpected, nil
	}
	for i := range meterSamples {
		l, r, err := mx.Meters()
		if err != nil {
			return silenceUnknown, err
		}
		if l > 0 || r > 0 {
			return silenceWedged, nil
		}
		if i < meterSamples-1 {
			time.Sleep(200 * time.Millisecond)
		}
	}
	return silenceGenuine, nil
}
