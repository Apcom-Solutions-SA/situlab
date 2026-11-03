# SituLab — Empaqueter et distribuer (Windows / macOS / Linux)

SituLab est multiplateforme. Chaque installeur se construit **sur l'OS cible** (le rendu WebView
et les formats d'installeur sont spécifiques à chaque système).

## Windows (fait)

Binaire **sous-système console** (indispensable pour que le mode CLI écrive de façon fiable) :

```bash
# Go ≥ 1.25 (directive go.mod), Wails v2 — toolchain système récent.
go-winres simply --icon build/appicon.png --product-version 1.8.0 --file-version 1.8.0
go build -tags "desktop,production" -ldflags "-X main.version=1.8.0 -s -w" -o build/bin/situlab.exe .
```

Installeur **NSIS** (raccourcis + désinstalleur) :

```bash
makensis build/installer.nsi      # -> build/SituLab-Setup.exe
```

## macOS

Sur un Mac (pas de problème de sous-système : la CLI et la GUI cohabitent dans un binaire normal) :

```bash
wails build -platform darwin/universal   # app universelle Intel+Apple Silicon
# -> build/bin/SituLab.app
# Paquet .dmg (optionnel) :
hdiutil create -volname SituLab -srcfolder build/bin/SituLab.app -ov -format UDZO build/SituLab.dmg
# Signature/notarisation (pour éviter Gatekeeper) : codesign + notarytool avec le compte Apple APCOM.
```

## Linux

Sur une distribution récente (dépendances : WebKitGTK, gtk3) :

```bash
wails build -platform linux/amd64         # -> build/bin/situlab
# AppImage (portable) via nfpm / linuxdeploy, ou paquet .deb :
#   nfpm pkg --packager deb --target build/situlab.deb   (avec un nfpm.yaml décrivant le binaire + .desktop + icône)
```

## CI multi-plateforme (GitHub Actions)

Les trois installeurs se construisent par CI : `.github/workflows/release.yml` (jobs `windows` /
`macos` / `linux`) s'exécute sur un tag `v*`, construit chaque OS sur son runner natif et publie les
artefacts sur la release GitHub. Pipeline à valider au premier run (signature/notarisation macOS non
encore branchée).

> Depuis ce poste **Windows**, seuls le binaire et l'installeur **Windows** sont produits. Les
> paquets macOS/Linux se construisent sur ces OS (ou via la CI ci-dessus).
