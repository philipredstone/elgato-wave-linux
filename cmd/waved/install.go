package main

import (
	_ "embed"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

//go:embed waved.service
var unitFile string

//go:embed 60-waved.rules
var udevRule string

const udevPath = "/etc/udev/rules.d/60-waved.rules"

func unitPath() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "systemd", "user", "waved.service")
}

// writes the user unit pointing at this binary and starts it. udev needs
// root, so that part is only checked and explained.
func install(out io.Writer) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return err
	}
	unit := strings.Replace(unitFile, "%h/.local/bin/waved", exe, 1)

	p := unitPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(p, []byte(unit), 0o644); err != nil {
		return err
	}
	fmt.Fprintln(out, "wrote", p)

	if err := systemctl("daemon-reload"); err != nil {
		return err
	}
	if err := systemctl("enable", "--now", "waved.service"); err != nil {
		return err
	}
	fmt.Fprintln(out, "waved.service enabled and started")

	if _, err := os.Stat(udevPath); errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(out, "\nno udev rule yet, the daemon can't talk to the device without it:\n\n"+
			"  waved udev-rule | sudo tee %s\n"+
			"  sudo udevadm control --reload && sudo udevadm trigger --subsystem-match=usb\n", udevPath)
	}
	return nil
}

func uninstall(out io.Writer) error {
	systemctl("disable", "--now", "waved.service")
	p := unitPath()
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	fmt.Fprintln(out, "removed", p)
	if err := systemctl("daemon-reload"); err != nil {
		return err
	}
	if _, err := os.Stat(udevPath); err == nil {
		fmt.Fprintf(out, "udev rule left in place, remove with: sudo rm %s\n", udevPath)
	}
	return nil
}

func systemctl(args ...string) error {
	cmd := exec.Command("systemctl", append([]string{"--user"}, args...)...)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("systemctl --user %s: %w", strings.Join(args, " "), err)
	}
	return nil
}
