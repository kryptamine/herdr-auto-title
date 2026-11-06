//go:build !windows

package main

import "context"

func desktopService(_ context.Context) error { return errUsage }
