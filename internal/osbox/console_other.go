// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

//go:build !windows

package osbox

import "os/exec"

// hideConsole : no-op hors Windows (pas de fenêtre console à masquer).
func hideConsole(cmd *exec.Cmd) {}
