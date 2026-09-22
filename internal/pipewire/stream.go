package pipewire

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"sync"
	"syscall"
	"time"
)

// one pw-cat process
type Stream struct {
	Name string
	cmd  *exec.Cmd
	done chan struct{}

	mu          sync.Mutex
	lastData    time.Time
	lastNonzero time.Time
}

const (
	startupGrace = 700 * time.Millisecond
	stopTimeout  = 500 * time.Millisecond
)

func Record(name string, src *Node) (*Stream, error) {
	s := newStream(name, "--record", src)
	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	s.cmd.Stdout = w
	err = s.start()
	w.Close()
	if err != nil {
		r.Close()
		return nil, err
	}
	go s.consume(r)
	return s.ready()
}

func PlaySilence(name string, sink *Node) (*Stream, error) {
	s := newStream(name, "--playback", sink, "--raw") // --raw or sndfile chokes on stdin
	zero, err := os.Open("/dev/zero")
	if err != nil {
		return nil, err
	}
	defer zero.Close()
	s.cmd.Stdin = zero
	if err := s.start(); err != nil {
		return nil, err
	}
	return s.ready()
}

func newStream(name, mode string, target *Node, extra ...string) *Stream {
	args := append([]string{
		mode,
		"--target", target.Name,
		// dont-reconnect: otherwise WirePlumber silently moves us to the default source
		"--properties", fmt.Sprintf(`{"node.name":%q,"node.dont-reconnect":true}`, name),
		"--channels", strconv.Itoa(target.Channels()),
		"--format", "s16",
		"--rate", "48000",
		"--latency", "200ms",
	}, extra...)
	cmd := exec.Command("pw-cat", append(args, "-")...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return &Stream{Name: name, cmd: cmd, done: make(chan struct{})}
}

func (s *Stream) start() error {
	if err := s.cmd.Start(); err != nil {
		return fmt.Errorf("pw-cat %s: %w", s.Name, err)
	}
	go func() {
		s.cmd.Wait()
		close(s.done)
	}()
	return nil
}

func (s *Stream) ready() (*Stream, error) {
	select {
	case <-s.done:
		return nil, fmt.Errorf("pw-cat %s exited on startup", s.Name)
	case <-time.After(startupGrace):
		return s, nil
	}
}

func (s *Stream) consume(r io.ReadCloser) {
	defer r.Close()
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			signal := slices.ContainsFunc(buf[:n], func(b byte) bool { return b != 0 })
			now := time.Now()
			s.mu.Lock()
			s.lastData = now
			if signal {
				s.lastNonzero = now
			}
			s.mu.Unlock()
		}
		if err != nil {
			return
		}
	}
}

func (s *Stream) Alive() bool {
	select {
	case <-s.done:
		return false
	default:
		return true
	}
}

func (s *Stream) SinceData() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return time.Since(s.lastData)
}

func (s *Stream) SinceSignal() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return time.Since(s.lastNonzero)
}

// pw-cat stuck in a stalled read ignores SIGTERM
func (s *Stream) Stop() error {
	if s == nil || s.cmd.Process == nil {
		return nil
	}
	pgid := s.cmd.Process.Pid
	for _, sig := range []syscall.Signal{syscall.SIGTERM, syscall.SIGKILL} {
		syscall.Kill(-pgid, sig)
		select {
		case <-s.done:
			return nil
		case <-time.After(stopTimeout):
		}
	}
	return fmt.Errorf("pw-cat %s does not exit (stuck in the kernel?)", s.Name)
}
