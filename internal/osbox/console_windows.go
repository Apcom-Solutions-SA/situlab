// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

//go:build windows

package osbox

import (
	"os/exec"
	"syscall"
)

// hideConsole empêche wsl.exe d'ouvrir une fenêtre console (qui, sous une app GUI, clignote à
// l'écran à chaque appel). CREATE_NO_WINDOW = 0x08000000.
func hideConsole(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}
