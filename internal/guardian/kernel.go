package guardian

import (
	"bufio"
	"context"
	"log/slog"
	"os/exec"
	"strings"
	"sync/atomic"
	"time"
)

// what snd-usb-audio logs once ep0 is dead (-110, sometimes -71)
var wedgeMessages = []string{"usb_set_interface failed", "cannot submit urb"}

const kernelRestartDelay = 10 * time.Second

// tails journalctl -k. needs journal access or it silently sees nothing
type kernelWatcher struct {
	log     *slog.Logger
	port    atomic.Pointer[string]
	lastErr atomic.Int64
}

func newKernelWatcher(log *slog.Logger) *kernelWatcher {
	return &kernelWatcher{log: log}
}

func (k *kernelWatcher) watch(port string) { k.port.Store(&port) }

func (k *kernelWatcher) errorWithin(d time.Duration) bool {
	last := k.lastErr.Load()
	return last != 0 && time.Since(time.Unix(0, last)) < d
}

func (k *kernelWatcher) run(ctx context.Context) {
	for {
		if err := k.follow(ctx); err != nil && ctx.Err() == nil {
			k.log.Warn("kernel log watcher stopped", "err", err)
		}
		if sleep(ctx, kernelRestartDelay) != nil {
			return
		}
	}
}

func (k *kernelWatcher) follow(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "journalctl", "--dmesg", "--follow", "--lines=0", "--output=cat")
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	sc := bufio.NewScanner(out)
	for sc.Scan() {
		if k.matches(sc.Text()) {
			k.lastErr.Store(time.Now().UnixNano())
		}
	}
	if err := sc.Err(); err != nil {
		cmd.Process.Kill()
		cmd.Wait()
		return err
	}
	return cmd.Wait()
}

func (k *kernelWatcher) matches(line string) bool {
	port := k.port.Load()
	if port == nil || !strings.Contains(line, "usb "+*port+":") {
		return false
	}
	for _, m := range wedgeMessages {
		if strings.Contains(line, m) {
			return true
		}
	}
	return false
}
