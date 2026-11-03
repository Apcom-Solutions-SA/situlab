# Contribuer à SituLab

Merci de l'intérêt porté au projet. Ce guide décrit le flux de travail et les exigences de qualité.

## Avant de commencer

- SituLab pilote le **git du système** (il ne le réimplémente pas) : installez `git`.
- Langage : **Go** (version de la directive `go.mod`) et **Wails v2** pour la fenêtre.
- La langue du code et des commentaires est le **français** (nomenclature maison : KERNEL, SPOKE, CORPUS…).

## Construire et lancer

```bash
go build -tags "desktop,production" -o build/bin/situlab.exe .
situlab                         # ouvre la fenêtre
situlab repo list --output json # mode agent (CLI)
situlab mcp                     # serveur MCP (stdio)
```

## Portes de qualité — toutes BLOQUANTES

L'intégration continue (`.github/workflows/ci.yml`) applique ces portes ; faites-les passer en local
avant d'ouvrir une demande de fusion :

```bash
gofmt -l .            # aucun écart de format (sur le contenu commité : le poste Windows commite en LF)
go vet ./...
golangci-lint run     # config v2 (.golangci.yml), jeu standard
go test -race ./...   # testé en CI sur Linux, Windows et macOS
govulncheck ./...     # zéro vulnérabilité appelée
```

Toute nouveauté s'accompagne de ses **tests**. Pas de refonte fonctionnelle non demandée dans une
contribution ciblée.

## Flux de travail

1. Forkez, créez une branche `work/<sujet>` courte et ciblée.
2. Commits **petits et logiques**, messages clairs (quoi et pourquoi).
3. Les cinq portes ci-dessus au vert.
4. Ouvrez une demande de fusion décrivant le changement et la vérification effectuée.

## Sécurité et secrets

- **Aucun secret** dans le dépôt (jeton, clé, mot de passe). Le trousseau et les accès vivent sur le poste.
- Une **vulnérabilité** se signale en privé (voir [`SECURITY.md`](SECURITY.md)), jamais en issue publique.

## Licence

En contribuant, vous acceptez que votre contribution soit distribuée sous la licence du projet
(**Apache-2.0**, voir [`LICENSE`](LICENSE) et [`NOTICE`](NOTICE)).
