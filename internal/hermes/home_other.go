//go:build !linux

package hermes

// ProcessHome is unavailable without a readable procfs startup environment.
// Configured homes and profile discovery work on every platform.
func ProcessHome(_ int) string { return "" }
