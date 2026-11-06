//go:build !windows

package autostart

import "os/exec" //nolint:depguard // keeps the service manager command unchanged on Unix

func hideWindow(_ *exec.Cmd) {}
