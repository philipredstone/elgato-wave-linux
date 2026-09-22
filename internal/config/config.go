package config

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/philipredstone/elgato-wave-linux/internal/usb"
)

const appName = "waved"

type Config struct {
	Device           *usb.ID // nil = any
	PlaybackPin      bool
	RememberSettings bool
	Notifications    bool
}

func Default() Config {
	return Config{
		PlaybackPin:      true,
		RememberSettings: true,
		Notifications:    true,
	}
}

func Load() (Config, error) {
	f, err := os.Open(Path())
	if errors.Is(err, fs.ErrNotExist) {
		return Default(), nil
	}
	if err != nil {
		return Default(), err
	}
	defer f.Close()
	return Parse(f)
}

func Parse(r io.Reader) (Config, error) {
	c := Default()
	var errs []error
	sc := bufio.NewScanner(r)
	for line := 1; sc.Scan(); line++ {
		text := strings.TrimSpace(sc.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		key, val, ok := strings.Cut(text, "=")
		if !ok {
			errs = append(errs, fmt.Errorf("line %d: want key = value", line))
			continue
		}
		if err := c.set(strings.TrimSpace(key), strings.TrimSpace(val)); err != nil {
			errs = append(errs, fmt.Errorf("line %d: %w", line, err))
		}
	}
	if err := sc.Err(); err != nil {
		errs = append(errs, err)
	}
	return c, errors.Join(errs...)
}

func (c *Config) set(key, val string) error {
	switch key {
	case "device":
		id, err := usb.ParseID(val)
		if err != nil {
			return err
		}
		c.Device = &id
		return nil
	case "playback_pin":
		return parseBool(val, &c.PlaybackPin)
	case "remember_settings", "remember_gain": // old name
		return parseBool(val, &c.RememberSettings)
	case "notifications":
		return parseBool(val, &c.Notifications)
	}
	return fmt.Errorf("unknown key %q", key)
}

func parseBool(s string, dst *bool) error {
	switch strings.ToLower(s) {
	case "yes", "on":
		*dst = true
		return nil
	case "no", "off":
		*dst = false
		return nil
	}
	b, err := strconv.ParseBool(s)
	if err != nil {
		return fmt.Errorf("%q is not a boolean", s)
	}
	*dst = b
	return nil
}

func Path() string { return filepath.Join(xdgDir("XDG_CONFIG_HOME", ".config"), appName, "config") }

func StateDir() string { return filepath.Join(xdgDir("XDG_STATE_HOME", ".local/state"), appName) }

func StatusFile() string {
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		dir = os.TempDir()
	}
	return filepath.Join(dir, appName+".status")
}

func xdgDir(env, fallback string) string {
	if dir := os.Getenv(env); dir != "" {
		return dir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, fallback)
}
