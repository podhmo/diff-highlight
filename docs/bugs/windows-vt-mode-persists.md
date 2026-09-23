# Windows: VT mode stays enabled after the process exits

`vt_windows.go` sets `ENABLE_VIRTUAL_TERMINAL_PROCESSING` on stdout and
stderr so ANSI colors render on native cmd.exe/PowerShell consoles.

Console output modes belong to the console's screen buffer, which is
shared with the parent shell and persists after this process exits.
There is deliberately no restore-on-exit: after running diff-highlight
in cmd.exe/PowerShell, that console keeps VT processing enabled, so a
later command writing a literal ESC sequence gets it interpreted
instead of displayed raw.

Reproduction (cmd.exe):

    diff-highlight < nul
    rem afterwards, a literal ESC byte printed by another command is
    rem interpreted as the start of an ANSI sequence

This is the same trade-off other Windows color tooling makes (e.g.
Python colorama's `just_fix_windows_console` also leaves the mode set —
an always-on VT console is usually what the user wants anyway). The
Perl oracle does not touch console modes at all, so this is a
Go-port-only consideration, not an inherited limitation. Restoration
would also only cover normal exits; a SIGKILL/crash leaves the mode set
regardless.

Decided: keep the mode, don't restore. Recorded per AGENTS.md's
`docs/bugs/` rule.
