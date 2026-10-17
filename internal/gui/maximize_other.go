//go:build !darwin && !windows

package gui

// Other systems keep the default window size.
func maximizeNative(any) bool { return true }
