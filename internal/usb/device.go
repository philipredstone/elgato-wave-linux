package usb

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const sysfsDevices = "/sys/bus/usb/devices"

type ID struct {
	Vendor, Product string
}

func ParseID(s string) (ID, error) {
	v, p, ok := strings.Cut(strings.ToLower(strings.TrimSpace(s)), ":")
	if !ok || !isHex4(v) || !isHex4(p) {
		return ID{}, fmt.Errorf("usb id %q: want vvvv:pppp", s)
	}
	return ID{v, p}, nil
}

func (id ID) String() string { return id.Vendor + ":" + id.Product }

func isHex4(s string) bool {
	_, err := strconv.ParseUint(s, 16, 16)
	return len(s) == 4 && err == nil
}

// one enumeration; replug or reset gives a new Node
type Device struct {
	ID     ID
	Port   string // "7-1.1"
	Node   string // "/dev/bus/usb/007/006"
	Serial string
}

func Find(ids ...ID) (*Device, bool) {
	entries, err := os.ReadDir(sysfsDevices)
	if err != nil {
		return nil, false
	}
	for _, want := range ids {
		for _, e := range entries {
			port := e.Name()
			if strings.Contains(port, ":") {
				continue
			}
			dir := filepath.Join(sysfsDevices, port)
			if (ID{attr(dir, "idVendor"), attr(dir, "idProduct")}) != want {
				continue
			}
			bus, err1 := strconv.Atoi(attr(dir, "busnum"))
			num, err2 := strconv.Atoi(attr(dir, "devnum"))
			if err1 != nil || err2 != nil {
				continue
			}
			return &Device{
				ID:     want,
				Port:   port,
				Node:   fmt.Sprintf("/dev/bus/usb/%03d/%03d", bus, num),
				Serial: attr(dir, "serial"),
			}, true
		}
	}
	return nil, false
}

func (d *Device) Present() bool {
	cur, ok := Find(d.ID)
	return ok && cur.Node == d.Node
}

// -1 until snd-usb-audio has bound
func (d *Device) ALSACard() int {
	cards, _ := filepath.Glob(filepath.Join(sysfsDevices, d.Port, d.Port+":*", "sound", "card*"))
	for _, c := range cards {
		if n, err := strconv.Atoi(strings.TrimPrefix(filepath.Base(c), "card")); err == nil {
			return n
		}
	}
	return -1
}

func attr(dir, name string) string {
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
