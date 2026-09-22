package mixer

import (
	"fmt"
	"strconv"
	"strings"
)

// Q8.8
func FormatQ8(raw int) string {
	return fmt.Sprintf("%.1f dB", float64(int16(raw))/256)
}

func ParseQ8(lo, hi float64) func(string) (int, error) {
	return func(s string) (int, error) {
		db, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(s), "dB"), 64)
		if err != nil || db < lo || db > hi {
			return 0, fmt.Errorf("want %g to %g dB", lo, hi)
		}
		return int(uint16(int16(db * 256))), nil
	}
}

func FormatBool(raw int) string {
	if raw != 0 {
		return "on"
	}
	return "off"
}

func ParseBool(s string) (int, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "on", "true", "yes", "1":
		return 1, nil
	case "off", "false", "no", "0":
		return 0, nil
	}
	return 0, fmt.Errorf("want on or off")
}
