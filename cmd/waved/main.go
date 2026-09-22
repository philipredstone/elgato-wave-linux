package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/philipredstone/elgato-wave-linux/internal/config"
	"github.com/philipredstone/elgato-wave-linux/internal/device"
	"github.com/philipredstone/elgato-wave-linux/internal/guardian"
	"github.com/philipredstone/elgato-wave-linux/internal/mixer"
)

var version = "dev" // -ldflags -X, see Makefile

const usage = `usage: waved <command> [args]

commands:
  daemon            guard the device (run from the systemd unit)
  status            show device and daemon state
  get               show mixer settings
  set <name> <val>  change a mixer setting
  meters            show input levels for two seconds
  devices           list supported devices
  version           print the version

configuration: ` + "~/.config/waved/config" + `
`

var errUsage = errors.New("usage")

func main() {
	err := run(os.Args[1:], os.Stdout)
	switch {
	case errors.Is(err, errUsage):
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	case err != nil:
		fmt.Fprintln(os.Stderr, "waved:", err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	if len(args) == 0 {
		return errUsage
	}
	cfg, cfgErr := config.Load()

	cmd, args := args[0], args[1:]
	switch cmd {
	case "daemon":
		return daemon(cfg, cfgErr)
	case "status":
		return status(out, selector(cfg))
	case "get":
		return get(out, selector(cfg))
	case "set":
		if len(args) != 2 {
			return errUsage
		}
		return set(out, selector(cfg), args[0], args[1])
	case "meters":
		return meters(out, selector(cfg))
	case "devices":
		return devices(out)
	case "version", "--version":
		fmt.Fprintln(out, "waved", version)
		return nil
	case "help", "--help", "-h":
		fmt.Fprint(out, usage)
		return nil
	}
	return errUsage
}

func selector(cfg config.Config) device.Selector {
	if cfg.Device != nil {
		return device.ByID(*cfg.Device)
	}
	return device.Any()
}

func daemon(cfg config.Config, cfgErr error) error {
	// no timestamps, journald has them
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	}))
	if cfgErr != nil {
		log.Warn("config", "path", config.Path(), "err", cfgErr)
	}
	log.Info("starting", "version", version,
		"playback_pin", cfg.PlaybackPin, "remember_settings", cfg.RememberSettings)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return guardian.New(cfg, selector(cfg), log).Run(ctx)
}

func findDevice(sel device.Selector) (*device.Device, error) {
	dev, ok := sel.Find()
	if !ok {
		return nil, errors.New("no supported device connected")
	}
	return dev, nil
}

func findMixer(sel device.Selector) (mixer.Mixer, error) {
	dev, err := findDevice(sel)
	if err != nil {
		return nil, err
	}
	if dev.Mixer == nil {
		return nil, fmt.Errorf("%s: control protocol not supported", dev.Model.Name)
	}
	return dev.Mixer, nil
}

func status(out io.Writer, sel device.Selector) error {
	if dev, ok := sel.Find(); ok {
		fmt.Fprintf(out, "device:   %s at usb %s, ALSA card %d\n", dev.Model.Name, dev.Port, dev.ALSACard())
		if dev.Mixer != nil {
			if info, err := dev.Mixer.Info(); err == nil {
				fmt.Fprintf(out, "firmware: %s, serial %s\n", info.Firmware, info.Serial)
			} else {
				fmt.Fprintf(out, "firmware: unreadable: %v\n", err)
			}
		}
	} else {
		fmt.Fprintln(out, "device:   not connected")
	}

	b, err := os.ReadFile(config.StatusFile())
	if err != nil {
		fmt.Fprintln(out, "daemon:   not running")
		return nil
	}
	fmt.Fprintf(out, "daemon:   %s\n", strings.TrimSpace(string(b)))
	return nil
}

func get(out io.Writer, sel device.Selector) error {
	mx, err := findMixer(sel)
	if err != nil {
		return err
	}
	s, err := mx.Read()
	if err != nil {
		return err
	}
	for _, f := range mx.Fields() {
		fmt.Fprintf(out, "%-6s %s\n", f.Name, f.Format(s[f.Name]))
	}
	return nil
}

func set(out io.Writer, sel device.Selector, name, value string) error {
	mx, err := findMixer(sel)
	if err != nil {
		return err
	}
	f, ok := mixer.Lookup(mx, name)
	if !ok || !f.Writable() {
		var b strings.Builder
		fmt.Fprintf(&b, "cannot set %q; settable:", name)
		for _, f := range mx.Fields() {
			if f.Writable() {
				fmt.Fprintf(&b, "\n  %-6s %s", f.Name, f.Usage)
			}
		}
		return errors.New(b.String())
	}
	raw, err := f.Parse(value)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if err := mx.Write(mixer.Settings{name: raw}); err != nil {
		return err
	}
	fmt.Fprintf(out, "%s %s\n", name, f.Format(raw))
	return nil
}

func meters(out io.Writer, sel device.Selector) error {
	mx, err := findMixer(sel)
	if err != nil {
		return err
	}
	for range 10 {
		l, r, err := mx.Meters()
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "L %10d  R %10d\n", l, r)
		time.Sleep(200 * time.Millisecond)
	}
	return nil
}

func devices(out io.Writer) error {
	for _, m := range device.Models {
		support := "stream guard only"
		if m.NewMixer != nil {
			support = "stream guard, mixer"
		}
		if !m.Tested {
			support += ", untested"
		}
		fmt.Fprintf(out, "%s  %-16s %s\n", m.ID, m.Name, support)
	}
	return nil
}
