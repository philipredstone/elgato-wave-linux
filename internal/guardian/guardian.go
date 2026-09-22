// Package guardian: capture first, then playback, keep both open, and
// fix things when the firmware wedges anyway. See NOTES.md.
package guardian

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"time"

	"github.com/philipredstone/elgato-wave-linux/internal/config"
	"github.com/philipredstone/elgato-wave-linux/internal/device"
	"github.com/philipredstone/elgato-wave-linux/internal/pipewire"
	"github.com/philipredstone/elgato-wave-linux/internal/state"
	"github.com/philipredstone/elgato-wave-linux/internal/systemd"
)

const (
	captureStreamName  = "waved-capture-pin"
	playbackStreamName = "waved-playback-pin"

	pollInterval = 2 * time.Second
)

var errNoMixer = errors.New("no mixer driver")

type Guardian struct {
	cfg      config.Config
	selector device.Selector
	log      *slog.Logger
	notifier *notifier
	kernel   *kernelWatcher
	settings *settingsSync
	ladder   ladder

	announced string
}

func New(cfg config.Config, sel device.Selector, log *slog.Logger) *Guardian {
	g := &Guardian{
		cfg:      cfg,
		selector: sel,
		log:      log,
		notifier: newNotifier(cfg.Notifications),
		kernel:   newKernelWatcher(log),
	}
	if cfg.RememberSettings {
		g.settings = &settingsSync{store: state.Store{Dir: config.StateDir()}, log: log}
	}
	return g
}

func (g *Guardian) Run(ctx context.Context) error {
	go g.kernel.run(ctx)
	go systemd.Watchdog(ctx)
	systemd.Notify("READY=1")
	defer g.setStatus("stopped")

	for {
		dev, src, sink, err := g.waitForDevice(ctx)
		if err != nil {
			return nil
		}
		g.kernel.watch(dev.Port)

		f := g.session(ctx, dev, src, sink)
		if ctx.Err() != nil {
			return nil
		}
		if err := g.recover(ctx, dev, f); err != nil {
			return nil
		}
	}
}

func (g *Guardian) waitForDevice(ctx context.Context) (*device.Device, *pipewire.Node, *pipewire.Node, error) {
	waiting := false
	for {
		if dev, ok := g.selector.Find(); ok {
			if graph, err := pipewire.Dump(); err == nil {
				if src, sink := graph.CardNodes(dev.ALSACard()); src != nil && sink != nil {
					g.announce(dev)
					return dev, src, sink, nil
				}
			}
		}
		if !waiting {
			g.log.Info("waiting for device")
			g.setStatus("waiting for device")
			waiting = true
		}
		if err := sleep(ctx, pollInterval); err != nil {
			return nil, nil, nil, err
		}
	}
}

func (g *Guardian) announce(dev *device.Device) {
	if dev.Node == g.announced {
		return
	}
	g.announced = dev.Node
	g.log.Info("device found", "model", dev.Model.Name, "port", dev.Port,
		"card", dev.ALSACard(), "mixer", dev.Mixer != nil)
}

// TODO status file goes stale on SIGKILL
func (g *Guardian) setStatus(s string) {
	systemd.Notify("STATUS=" + s)
	if err := os.WriteFile(config.StatusFile(), []byte(s+"\n"), 0o644); err != nil {
		g.log.Warn("write status file", "err", err)
	}
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
