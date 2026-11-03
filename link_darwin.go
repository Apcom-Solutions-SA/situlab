// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package main

// Sur macOS, le runtime Wails référence la classe Objective-C UTType (framework
// UniformTypeIdentifiers, disponible depuis macOS 11). L'outil « wails build » ajoute ce framework à
// l'édition de liens, mais une compilation native brute (go build / go run, utilisée par le smoke CI
// et en développement local sur Mac) ne voit que les directives cgo du paquet wails, qui l'omettent
// en v2.16. On le fournit donc explicitement ici pour que l'édition de liens aboutisse sans passer
// par « wails build » (symbole manquant sinon : _OBJC_CLASS_$_UTType). Fichier restreint à darwin par
// le suffixe _darwin.go : aucun effet sur les compilations Windows et Linux.

/*
#cgo LDFLAGS: -framework UniformTypeIdentifiers
*/
import "C"
