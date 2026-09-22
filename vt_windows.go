//go:build windows

package main

import (
	"os"

	"golang.org/x/sys/windows"
)

// enableVirtualTerminal turns on ENABLE_VIRTUAL_TERMINAL_PROCESSING on
// console handles so ANSI colors render on cmd.exe/PowerShell instead of
// printing raw escapes. Handles that are not consoles (pipes, files, and
// MSYS2/git-bash ptys, which already speak ANSI) fail GetConsoleMode and
// are left alone. The Perl oracle does not do this; it is a Go-port-only
// nicety for native Windows consoles.
func enableVirtualTerminal() {
	for _, f := range []*os.File{os.Stdout, os.Stderr} {
		h := windows.Handle(f.Fd())
		var mode uint32
		if err := windows.GetConsoleMode(h, &mode); err != nil {
			continue
		}
		_ = windows.SetConsoleMode(h, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING)
	}
}
