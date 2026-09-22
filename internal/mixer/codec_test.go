package mixer

import "testing"

func TestQ8RoundTrip(t *testing.T) {
	parse := ParseQ8(-128, 75)
	for in, want := range map[string]string{
		"56.25": "56.2 dB",
		"0":     "0.0 dB",
		"-12.5": "-12.5 dB",
		"75dB":  "75.0 dB",
	} {
		raw, err := parse(in)
		if err != nil {
			t.Fatalf("%s: %v", in, err)
		}
		if got := FormatQ8(raw); got != want {
			t.Errorf("%s -> %s, want %s", in, got, want)
		}
	}
}

func TestParseQ8Range(t *testing.T) {
	parse := ParseQ8(0, 75)
	for _, in := range []string{"-1", "75.1", "loud", ""} {
		if _, err := parse(in); err == nil {
			t.Errorf("%q: want error", in)
		}
	}
}

func TestParseBool(t *testing.T) {
	for in, want := range map[string]int{"on": 1, "OFF": 0, "yes": 1, "0": 0} {
		if got, err := ParseBool(in); err != nil || got != want {
			t.Errorf("%q = %d, %v; want %d", in, got, err, want)
		}
	}
	if _, err := ParseBool("maybe"); err == nil {
		t.Error("want error for maybe")
	}
}
