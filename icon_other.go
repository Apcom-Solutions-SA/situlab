// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

//go:build !windows

package main

// setWindowIcon : sans objet hors Windows (macOS/Linux posent l'icone via les options Wails / le .desktop).
func setWindowIcon() {}
