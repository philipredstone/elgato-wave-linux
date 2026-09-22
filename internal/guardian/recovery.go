package guardian

import (
	"context"
	"os/exec"
	"time"

	"github.com/philipredstone/elgato-wave-linux/internal/device"
)

const (
	// one reset per window, more makes it worse (NOTES.md)
	resetWindow       = 10 * time.Minute
	reenumerateWait   = 20 * time.Second
	wireplumberSettle = 4 * time.Second
	replugReminders   = 5
	probeInterval     = 2 * time.Minute
)

type ladder struct {
	softFailures int
	lastReset    time.Time
}

func (l *ladder) healthy() { l.softFailures = 0 }

func (g *Guardian) recover(ctx context.Context, dev *device.Device, f fault) error {
	if f.kind == faultGone {
		g.log.Info("device disconnected")
		g.setStatus("device disconnected")
		g.notifier.send(urgencyNormal, dev.Model.Name+" disconnected")
		return nil
	}
	if f.kind == faultHard {
		g.log.Warn("hard fault", "reason", f.reason)
		g.ladder.softFailures = 0
		return g.resetOrReplug(ctx, dev)
	}

	g.ladder.softFailures++
	switch g.ladder.softFailures {
	case 1:
		g.log.Warn("recovery: restarting streams", "reason", f.reason)
		return sleep(ctx, pollInterval)
	case 2:
		g.log.Warn("recovery: restarting WirePlumber", "reason", f.reason)
		return g.restartWirePlumber(ctx)
	default:
		g.log.Warn("recovery: resetting device", "reason", f.reason)
		g.ladder.softFailures = 0
		return g.resetOrReplug(ctx, dev)
	}
}

// full PCM close/reopen, fixes the stall variant by itself
func (g *Guardian) restartWirePlumber(ctx context.Context) error {
	if err := exec.CommandContext(ctx, "systemctl", "--user", "restart", "wireplumber").Run(); err != nil {
		g.log.Error("restart WirePlumber", "err", err)
	}
	return sleep(ctx, wireplumberSettle)
}

func (g *Guardian) resetOrReplug(ctx context.Context, dev *device.Device) error {
	if !g.ladder.lastReset.IsZero() && time.Since(g.ladder.lastReset) < resetWindow {
		return g.awaitReplug(ctx, dev)
	}
	g.ladder.lastReset = time.Now()

	g.log.Warn("resetting USB port", "port", dev.Port)
	g.setStatus("resetting device")
	g.notifier.send(urgencyNormal, dev.Model.Name+" is not responding, resetting it")
	if err := dev.Reset(); err != nil {
		g.log.Error("USB reset", "err", err)
		return g.awaitReplug(ctx, dev)
	}

	for deadline := time.Now().Add(reenumerateWait); time.Now().Before(deadline); {
		if err := sleep(ctx, pollInterval); err != nil {
			return err
		}
		if _, ok := g.selector.Find(); ok {
			g.log.Info("device re-enumerated")
			return g.restartWirePlumber(ctx)
		}
	}
	g.log.Warn("device did not re-enumerate after reset")
	return g.awaitReplug(ctx, dev)
}

// waits for a new enumeration. every couple of minutes it also asks ep0
// once, in case the "hard fault" was a false alarm and the device is fine.
func (g *Guardian) awaitReplug(ctx context.Context, dev *device.Device) error {
	g.log.Warn("waiting for the device to be replugged")
	g.setStatus("replug needed")
	msg := dev.Model.Name + " has crashed: unplug it and plug it back in"

	lastProbe := time.Now()
	for reminders := 0; ; {
		cur, ok := g.selector.Find()
		if ok && cur.Node != dev.Node {
			g.log.Info("device replugged")
			g.notifier.send(urgencyNormal, dev.Model.Name+" reconnected")
			g.ladder.lastReset = time.Time{}
			return g.restartWirePlumber(ctx)
		}
		if ok && reminders < replugReminders && g.notifier.send(urgencyCritical, msg) {
			reminders++
		}
		if ok && dev.Mixer != nil && time.Since(lastProbe) >= probeInterval {
			lastProbe = time.Now()
			if _, err := dev.Mixer.Info(); err == nil {
				g.log.Info("control endpoint answers again, retrying")
				return nil
			}
		}
		if err := sleep(ctx, pollInterval); err != nil {
			return err
		}
	}
}
