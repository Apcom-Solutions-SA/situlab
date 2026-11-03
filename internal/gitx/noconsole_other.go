// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

//go:build !windows

package gitx

import "os/exec"

func hideProcess(cmd *exec.Cmd) {}
