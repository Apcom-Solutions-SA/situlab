// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

//go:build !windows

package main

// releaseOwnConsole ne fait rien hors Windows (pas de sous-système console à gérer).
func releaseOwnConsole() {}
