package guardian

import "testing"

func TestKernelMatches(t *testing.T) {
	k := newKernelWatcher(nil)
	k.watch("7-1.1")
	for line, want := range map[string]bool{
		"usb 7-1.1: usb_set_interface failed (-110)":               true,
		"usb 7-1.1: cannot submit urb (err = -71)":                 true,
		"usb 7-1.1: usb_set_interface failed (-19)":                false, // unplug
		"usb 7-1.1: cannot submit urb (err = -28)":                 false, // bandwidth
		"usb 7-1.2: usb_set_interface failed (-110)":               false, // other port
		"usb 7-1.1: new full-speed USB device number 9 using xhci": false,
	} {
		if got := k.matches(line); got != want {
			t.Errorf("%q: got %v, want %v", line, got, want)
		}
	}
}
