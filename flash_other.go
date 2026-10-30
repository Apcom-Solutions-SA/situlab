// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

//go:build !windows

package main

// flashTaskbarAttention : pas d'opération hors Windows. Wails v2 n'expose pas d'API portable de
// clignotement d'icône ; sur macOS l'équivalent serait requestUserAttention(NSInformationalRequest)
// et sur Linux l'« urgency hint » WM — à câbler par OS si le besoin se confirme (retour #15). Le
// « cling » sonore, lui, fonctionne partout (Web Audio, côté frontend).
func flashTaskbarAttention() {}
