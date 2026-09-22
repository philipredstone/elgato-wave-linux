package guardian

import (
	"context"
	"fmt"
	"time"

	"github.com/philipredstone/elgato-wave-linux/internal/device"
	"github.com/philipredstone/elgato-wave-linux/internal/pipewire"
	"github.com/philipredstone/elgato-wave-linux/internal/usb"
)

const (
	tick = time.Second

	firstDataTimeout = 10 * time.Second
	firstSignalWait  = 3 * time.Second
	stallTimeout     = 3 * time.Second
	silenceTimeout   = 12 * time.Second
	silenceRecheck   = time.Minute
	linkInterval     = 10 * time.Second
	settingsInterval = 5 * time.Second
	presenceInterval = 5 * time.Second
	kernelErrorTTL   = 10 * time.Second
)

type fault struct {
	kind   faultKind
	reason string
}

type faultKind int

const (
	faultStartup faultKind = iota
	faultSoft
	faultHard
	faultGone
)

func soft(format string, args ...any) fault {
	return fault{faultSoft, fmt.Sprintf(format, args...)}
}

type session struct {
	g        *Guardian
	dev      *device.Device
	src      *pipewire.Node
	sink     *pipewire.Node
	capture  *pipewire.Stream
	playback *pipewire.Stream
}

func (g *Guardian) session(ctx context.Context, dev *device.Device, src, sink *pipewire.Node) fault {
	s := &session{g: g, dev: dev, src: src, sink: sink}
	defer s.stop()

	if err := s.start(ctx); err != nil {
		return fault{faultStartup, err.Error()}
	}
	g.ladder.healthy()
	g.log.Info("healthy")
	g.setStatus("healthy")

	// not before the streams are up, writes during attach can wedge it
	if g.settings != nil {
		g.settings.restore(dev)
	}
	return s.supervise(ctx)
}

func (s *session) start(ctx context.Context) error {
	var err error
	if s.capture, err = pipewire.Record(captureStreamName, s.src); err != nil {
		return err
	}

	for deadline := time.Now().Add(firstDataTimeout); s.capture.SinceData() > time.Second; {
		if !s.capture.Alive() {
			return fmt.Errorf("capture stream exited")
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("no capture data within %s", firstDataTimeout)
		}
		if err := sleep(ctx, 500*time.Millisecond); err != nil {
			return err
		}
	}

	for deadline := time.Now().Add(firstSignalWait); s.capture.SinceSignal() > time.Second && time.Now().Before(deadline); {
		if err := sleep(ctx, 300*time.Millisecond); err != nil {
			return err
		}
	}
	if s.capture.SinceSignal() > time.Second {
		switch v, err := checkSilence(s.dev); v {
		case silenceWedged:
			return fmt.Errorf("capture is silent but the input meters show signal")
		case silenceUnknown:
			if usb.IsFatal(err) {
				return fmt.Errorf("capture is silent and the control endpoint fails: %w", err)
			}
		}
	}

	if s.g.cfg.PlaybackPin {
		if s.playback, err = pipewire.PlaySilence(playbackStreamName, s.sink); err != nil {
			s.g.log.Warn("playback stream failed, continuing with capture only", "err", err)
		}
	}
	return nil
}

func (s *session) stop() {
	for _, st := range []*pipewire.Stream{s.playback, s.capture} {
		if err := st.Stop(); err != nil {
			s.g.log.Error("stop stream", "err", err)
		}
	}
}

func (s *session) supervise(ctx context.Context) fault {
	var (
		g              = s.g
		now            = time.Now()
		lastPresence   = now
		lastLinkCheck  = now
		lastSettings   = now
		lastSilenceChk time.Time
	)
	for {
		if err := sleep(ctx, tick); err != nil {
			return fault{faultGone, "shutdown"}
		}
		now = time.Now()

		// TODO presence check should come first, an unplug logs urb errors too
		if g.kernel.errorWithin(kernelErrorTTL) {
			return fault{faultHard, "kernel reports USB interface errors"}
		}

		if now.Sub(lastPresence) >= presenceInterval {
			lastPresence = now
			if !s.dev.Present() {
				return fault{faultGone, "device disconnected"}
			}
		}

		if !s.capture.Alive() {
			return soft("capture stream exited")
		}
		if d := s.capture.SinceData(); d > stallTimeout {
			return soft("capture stalled for %s", d.Round(100*time.Millisecond))
		}
		if s.playback != nil && !s.playback.Alive() {
			g.log.Warn("playback stream exited, restarting it")
			var err error
			if s.playback, err = pipewire.PlaySilence(playbackStreamName, s.sink); err != nil {
				g.log.Warn("restart playback stream", "err", err)
			}
		}

		if d := s.capture.SinceSignal(); d > silenceTimeout && now.Sub(lastSilenceChk) >= silenceRecheck {
			lastSilenceChk = now
			v, err := checkSilence(s.dev)
			switch {
			case v == silenceWedged:
				return soft("capture silent for %s while the input meters show signal", d.Round(time.Second))
			case v == silenceUnknown && usb.IsFatal(err):
				return fault{faultHard, "capture silent and control endpoint fails: " + err.Error()}
			case v == silenceGenuine:
				g.log.Info("input is silent", "for", d.Round(time.Second))
			}
		}

		if g.settings != nil && now.Sub(lastSettings) >= settingsInterval {
			lastSettings = now
			g.settings.poll(s.dev)
		}

		if now.Sub(lastLinkCheck) >= linkInterval {
			lastLinkCheck = now
			graph, err := pipewire.Dump()
			if err != nil {
				continue
			}
			if !graph.Linked(s.src.Name, captureStreamName) {
				return soft("capture stream is no longer linked to %s", s.src.Name)
			}
		}
	}
}
