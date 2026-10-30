// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

//go:build windows

package gitx

import (
	"os/exec"
	"syscall"
)

// CREATE_NO_WINDOW empêche l'allocation d'une console pour chaque sous-processus git.
// Sans cela, un process GUI (sans console) qui lance des dizaines de `git` paie une
// allocation/destruction de console par appel — lent, sérialisé, et source de flashs.
const createNoWindow = 0x08000000

func hideProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}
