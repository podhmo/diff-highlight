//go:build !windows

package main

// enableVirtualTerminal is a no-op outside Windows; ANSI escapes work on
// every supported non-Windows target already.
func enableVirtualTerminal() {}
