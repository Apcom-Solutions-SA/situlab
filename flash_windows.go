// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

// flashTaskbarAttention fait clignoter l'icône de SituLab dans la barre des tâches (retour #15) quand
// une demande de PIN surgit : utile lorsque la fenêtre n'est pas au premier plan (action CLI/MCP).
// Best-effort — toute erreur est avalée. Wails v2 n'expose pas le handle natif ; on retrouve la fenêtre
// par son titre ("SituLab", figé dans wails.json). FlashWindowEx avec FLASHW_TIMERNOFG ne clignote QUE
// si la fenêtre n'est pas déjà au premier plan : l'appel est donc auto-neutralisé quand l'utilisateur
// est déjà devant l'app (le cas in-app passe de toute façon par pinConfirm, pas par ce chemin).
func flashTaskbarAttention() {
	defer func() { _ = recover() }()
	user32 := syscall.NewLazyDLL("user32.dll")
	findWindow := user32.NewProc("FindWindowW")
	flashWindowEx := user32.NewProc("FlashWindowEx")

	title, err := syscall.UTF16PtrFromString("SituLab")
	if err != nil {
		return
	}
	hwnd, _, _ := findWindow.Call(0, uintptr(unsafe.Pointer(title)))
	if hwnd == 0 {
		return
	}

	// FLASHWINFO : voir MSDN. dwFlags = FLASHW_ALL (0x3) | FLASHW_TIMERNOFG (0xC) = 0xF → clignote
	// (barre des tâches + titre) jusqu'au passage de la fenêtre au premier plan. uCount = 0 avec
	// TIMERNOFG : le système gère l'arrêt tout seul au focus.
	type flashwinfo struct {
		cbSize    uint32
		hwnd      uintptr
		dwFlags   uint32
		uCount    uint32
		dwTimeout uint32
	}
	fi := flashwinfo{
		dwFlags: 0x0000000F,
		hwnd:    hwnd,
	}
	fi.cbSize = uint32(unsafe.Sizeof(fi))
	flashWindowEx.Call(uintptr(unsafe.Pointer(&fi)))
}
