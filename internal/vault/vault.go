// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

// Package vault range les tokens d'instance dans le coffre natif de l'OS
// (Windows Credential Manager, macOS Keychain, Secret Service Linux) via go-keyring.
// Aucun token n'est jamais écrit en clair sur le disque.
package vault

import "github.com/zalando/go-keyring"

const service = "SituLab"

// SetToken range (ou remplace) le token d'une instance.
func SetToken(instance, token string) error {
	return keyring.Set(service, instance, token)
}

// GetToken lit le token d'une instance.
func GetToken(instance string) (string, error) {
	return keyring.Get(service, instance)
}

// DeleteToken retire le token d'une instance du coffre.
func DeleteToken(instance string) error {
	err := keyring.Delete(service, instance)
	if err == keyring.ErrNotFound {
		return nil
	}
	return err
}
