package systemd

import (
	"context"
	"net"
	"os"
	"strconv"
	"time"
)

func Notify(state string) error {
	sock := os.Getenv("NOTIFY_SOCKET")
	if sock == "" {
		return nil
	}
	conn, err := net.Dial("unixgram", sock)
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = conn.Write([]byte(state))
	return err
}

func Watchdog(ctx context.Context) {
	usec, err := strconv.ParseInt(os.Getenv("WATCHDOG_USEC"), 10, 64)
	if err != nil || usec <= 0 {
		return
	}
	t := time.NewTicker(time.Duration(usec) * time.Microsecond / 2)
	defer t.Stop()
	for {
		Notify("WATCHDOG=1")
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
