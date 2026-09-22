package config

import (
	"strings"
	"testing"

	"github.com/philipredstone/elgato-wave-linux/internal/usb"
)

func TestParse(t *testing.T) {
	c, err := Parse(strings.NewReader(`
# comment
device = 0FD9:007D
playback_pin = no
remember_gain = false
`))
	if err != nil {
		t.Fatal(err)
	}
	if c.Device == nil || *c.Device != (usb.ID{Vendor: "0fd9", Product: "007d"}) {
		t.Errorf("device = %v", c.Device)
	}
	if c.PlaybackPin || c.RememberSettings || !c.Notifications {
		t.Errorf("got %+v", c)
	}
}

func TestParseReportsBadLinesButKeepsGoodOnes(t *testing.T) {
	c, err := Parse(strings.NewReader("gain = 60\nplayback_pin = maybe\nnotifications = no\nnonsense\n"))
	if err == nil {
		t.Fatal("want error")
	}
	for _, want := range []string{"line 1", "line 2", "line 4"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q lacks %q", err, want)
		}
	}
	if c.Notifications || !c.PlaybackPin {
		t.Errorf("got %+v", c)
	}
}
