// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

//go:build windows

package main

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// releaseOwnConsole est appelé en mode GUI. SituLab est compilé en sous-système
// « console » pour que le mode CLI écrive de façon fiable (le shell attend le
// process et capture stdout). En contrepartie, un lancement par double-clic alloue
// une fenêtre console : si SituLab en est le seul propriétaire, on la libère
// immédiatement pour ne pas laisser de fenêtre noire derrière l'application.
// Lancé depuis un shell existant (console partagée), on n'y touche pas.
func releaseOwnConsole() {
	k32 := windows.NewLazySystemDLL("kernel32.dll")
	getList := k32.NewProc("GetConsoleProcessList")
	freeConsole := k32.NewProc("FreeConsole")

	var pids [8]uint32
	n, _, _ := getList.Call(uintptr(unsafe.Pointer(&pids[0])), uintptr(len(pids)))
	if n <= 1 { // console allouée pour nous seuls -> on la libère
		freeConsole.Call()
	}
}
