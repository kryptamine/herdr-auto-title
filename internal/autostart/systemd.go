package autostart

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// systemdUnit names the user service.
const systemdUnit = name + ".service"

// systemd starts the desktop mode as a service of the user's systemd manager.
type systemd struct{ host }

// xdg is the directory an XDG variable names, or fallback under home when it
// is unset or relative, which the specification says to ignore.
func (s systemd) xdg(variable string, fallback ...string) string {
	if dir := s.getenv(variable); filepath.IsAbs(dir) {
		return dir
	}

	return filepath.Join(append([]string{s.home}, fallback...)...)
}

func (s systemd) binary() string {
	return filepath.Join(s.xdg("XDG_DATA_HOME", ".local", "share"), name, name)
}

func (s systemd) unit() string {
	return filepath.Join(s.xdg("XDG_CONFIG_HOME", ".config"), "systemd", "user", systemdUnit)
}

func (s systemd) install(ctx context.Context, exe string) (Installed, error) {
	if err := copyBinary(exe, s.binary()); err != nil {
		return Installed{}, fmt.Errorf("copy the binary: %w", err)
	}

	unit, err := render("systemd.service", systemdService{Binary: s.binary()})
	if err == nil {
		err = writeFile(s.unit(), unit)
	}

	if err != nil {
		return Installed{}, fmt.Errorf("write the unit: %w", err)
	}

	// restart, not start: a service already running is still the old copy.
	for _, args := range [][]string{
		{"daemon-reload"},
		{"enable", systemdUnit},
		{"restart", systemdUnit},
	} {
		if err := s.run(ctx, "systemctl", append([]string{"--user"}, args...)...); err != nil {
			return Installed{}, err
		}
	}

	return Installed{Binary: s.binary(), Service: s.unit()}, nil
}

func (s systemd) uninstall(ctx context.Context) error {
	// With no unit there is no service to stop, and no systemctl to ask.
	if _, err := os.Stat(s.unit()); errors.Is(err, os.ErrNotExist) {
		return removeBinary(s.binary())
	}

	if err := s.run(ctx, "systemctl", "--user", "disable", "--now", systemdUnit); err != nil {
		return err
	}

	if err := remove(s.unit()); err != nil {
		return err
	}

	if err := removeBinary(s.binary()); err != nil {
		return err
	}

	return s.run(ctx, "systemctl", "--user", "daemon-reload")
}
