// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

//go:build windows

package main

import (
	_ "embed"
	"os"
	"path/filepath"
	"syscall"
	"time"
	"unsafe"
)

//go:embed build/windows/icon.ico
var windowIconICO []byte

// setWindowIcon pose l'icone SituLab sur la fenetre (barre de titre + barre des taches). Wails v2 ne
// pose pas l'icone de fenetre sous Windows (seule l'icone d'exe/ressource est embarquee, que le shell
// affiche) : on la met donc nous-memes via WM_SETICON + SetClassLongPtr. La fenetre se retrouve par son
// titre, comme flashTaskbarAttention. L'icone embarquee est ecrite en fichier temporaire car LoadImageW
// charge depuis un .ico fichier. Best-effort : toute erreur est avalee. A appeler apres domReady (la
// fenetre doit exister) ; la boucle couvre le court delai d'apparition.
func setWindowIcon() {
	defer func() { _ = recover() }()
	if len(windowIconICO) == 0 {
		return
	}
	tmp := filepath.Join(os.TempDir(), "situlab-window.ico")
	if err := os.WriteFile(tmp, windowIconICO, 0o600); err != nil {
		return
	}
	tmpPtr, err := syscall.UTF16PtrFromString(tmp)
	if err != nil {
		return
	}
	title, err := syscall.UTF16PtrFromString("SituLab")
	if err != nil {
		return
	}

	user32 := syscall.NewLazyDLL("user32.dll")
	findWindow := user32.NewProc("FindWindowW")
	loadImage := user32.NewProc("LoadImageW")
	sendMessage := user32.NewProc("SendMessageW")
	setClassLong := user32.NewProc("SetClassLongPtrW")

	const (
		imageIcon     = 1
		lrLoadFile    = 0x00000010
		lrDefaultSize = 0x00000040
		wmSetIcon     = 0x0080
		iconSmall     = 0
		iconBig       = 1
		gclpHIcon     = ^uintptr(13) // -14 (GCLP_HICON)
		gclpHIconSm   = ^uintptr(33) // -34 (GCLP_HICONSM)
	)

	var hwnd uintptr
	for i := 0; i < 50; i++ {
		h, _, _ := findWindow.Call(0, uintptr(unsafe.Pointer(title)))
		if h != 0 {
			hwnd = h
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if hwnd == 0 {
		return
	}

	hBig, _, _ := loadImage.Call(0, uintptr(unsafe.Pointer(tmpPtr)), imageIcon, 0, 0, lrLoadFile|lrDefaultSize)
	hSmall, _, _ := loadImage.Call(0, uintptr(unsafe.Pointer(tmpPtr)), imageIcon, 16, 16, lrLoadFile)
	if hBig != 0 {
		sendMessage.Call(hwnd, wmSetIcon, iconBig, hBig)
		setClassLong.Call(hwnd, gclpHIcon, hBig)
	}
	if hSmall != 0 {
		sendMessage.Call(hwnd, wmSetIcon, iconSmall, hSmall)
		setClassLong.Call(hwnd, gclpHIconSm, hSmall)
	}
}
