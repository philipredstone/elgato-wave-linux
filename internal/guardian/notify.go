package guardian

import (
	"os/exec"
	"sync"
	"time"
)

type urgency string

const (
	urgencyNormal   urgency = "normal"
	urgencyCritical urgency = "critical"
)

const notifyRepeatInterval = 2 * time.Minute

type notifier struct {
	enabled bool
	mu      sync.Mutex
	sent    map[string]time.Time
}

func newNotifier(enabled bool) *notifier {
	return &notifier{enabled: enabled, sent: map[string]time.Time{}}
}

func (n *notifier) send(u urgency, msg string) bool {
	if !n.enabled {
		return false
	}
	n.mu.Lock()
	if time.Since(n.sent[msg]) < notifyRepeatInterval {
		n.mu.Unlock()
		return false
	}
	n.sent[msg] = time.Now()
	n.mu.Unlock()
	return exec.Command("notify-send", "--app-name=waved", "--urgency="+string(u), msg).Run() == nil
}
