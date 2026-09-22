package state

import (
	"errors"
	"maps"
	"os"
	"path/filepath"
	"testing"

	"github.com/philipredstone/elgato-wave-linux/internal/mixer"
)

func TestRoundTrip(t *testing.T) {
	s := Store{Dir: filepath.Join(t.TempDir(), "state")}
	if _, err := s.Load("dev"); !errors.Is(err, ErrNone) {
		t.Fatalf("empty store: %v", err)
	}
	want := mixer.Settings{"gain": 14400, "mute": 0, "hp": 0xf400}
	if err := s.Save("dev", want); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load("dev")
	if err != nil {
		t.Fatal(err)
	}
	if !maps.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	entries, _ := os.ReadDir(s.Dir)
	if len(entries) != 1 {
		t.Errorf("temp files left behind: %v", entries)
	}
}
