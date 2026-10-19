# SituLab

**L'utilitaire git et mémoire pour tous.** Un poste de travail de bureau, multiplateforme, qui rend
lisible ce que vous avez en ligne et en local — sans ouvrir un navigateur, sans connaître git en
profondeur — et qui garde votre mémoire de travail en ordre pour vous comme pour vos agents IA.

> SituLab — © 2026 APCOM Solutions SA — version 1.16.2 — licence Apache 2.0

## Quatre piliers

- **KERNEL** — le dépôt qui *est* votre mémoire : désigné, lisible, synchronisé en deux gestes.
- **SPOKES** — les projections de mission rattachées à un KERNEL.
- **CORPUS** — vos clouds de fichiers souverains (Nextcloud, kDrive), avec un accès IA borné par dossier.
- **BACKUP** — vos plans de sauvegarde des dépôts vers le CORPUS.

Autour : l'**appairage** *clone local ↔ atelier ↔ vitrine* et la **promotion** d'une version validée.

## Idée

Vous vous connectez à une ou plusieurs instances **Forgejo/Gitea** (ex. `https://git.apcom.app`)
ou à **GitHub**, et à vos **clouds souverains**. SituLab vous montre alors, **selon vos droits**, vos
dépôts et vos fichiers, et pilote les **opérations git locales** (statut, clone, push) comme les gestes
cloud. Simple pour l'humain, scriptable pour les agents IA.

Pour Forgejo, l'onboarding crée lui-même un jeton à partir de vos identifiants (seul le PIN est
ensuite redemandé). Pour GitHub, vous collez un **jeton d'accès personnel** (GitHub ne permet pas
à une application de le créer) ; la gestion des comptes et des jetons reste propre à Forgejo.

## Trois accès, un seul noyau

- **Fenêtre graphique** (humain) — application de bureau native.
- **Ligne de commande** `situlab` (agents en shell) — mêmes opérations dépôts et git que la fenêtre, via le même noyau ; les réglages (rôles, politique PIN, projets) restent réservés à la fenêtre.
- **Serveur MCP** `situlab mcp` (agents sans terminal : Cursor, Claude Desktop…) — mêmes capacités, même consentement PIN.

Un seul binaire : sans argument il ouvre la fenêtre, avec une commande il agit en mode agent.

```
situlab                         # ouvre la fenêtre
situlab repo list --output json # mode agent (CLI)
situlab mcp                     # serveur MCP (stdio)
```

Aucune logique métier n'est dupliquée : les trois accès s'appuient sur le même noyau
(`internal/core`), et toute écriture distante passe par le même point de consentement PIN.

## Sécurité

- **Tokens chiffrés par le PIN** (trousseau local : Argon2id → KEK → DEK, AES-256-GCM) — plus de
  coffre OS (migré automatiquement depuis une version antérieure). Jamais en clair sur le disque ;
  le PIN n'est stocké nulle part (il *est* la clé).
- **PIN utilisateur obligatoire pour tout push distant** : un agent prépare l'action, l'humain
  seul débloque — via une fenêtre de saisie sur le poste ; le PIN ne transite jamais par l'agent.

## Pilotage par un agent IA

Toute la surface est documentée pour le pilotage par une IA — elle prépare les actions, l'humain
valide les écritures par son PIN : voir [`docs/AGENTS.md`](docs/AGENTS.md).

## Prérequis

- **git** installé sur la machine (SituLab pilote le git du système, il ne le réimplémente pas).
- Windows : WebView2 (présent par défaut sur Windows 11).

## Construire depuis les sources

```bash
# Go ≥ 1.25 (directive go.mod), Wails v2. Toolchain système récent : rien à épingler.

# Icône Windows (ressource)
go-winres simply --icon build/appicon.png --product-version 1.16.2 --file-version 1.16.2

# Binaire console (indispensable pour que le mode CLI écrive de façon fiable)
go build -tags "desktop,production" -ldflags "-X main.version=1.16.2 -s -w" -o build/bin/situlab.exe .

# Installateur Windows (NSIS) : depuis build/
cd build && makensis installer.nsi   # -> build/SituLab-Setup.exe
```

### Installateurs multi-OS

Les installateurs **Windows / macOS / Linux** se produisent de façon reproductible par CI :
`.github/workflows/release.yml` (GitHub Actions) bâtit les trois sur un tag `v*`. Chaque OS a
sa stack native (Wails = toolchain par OS) : Windows → binaire console + NSIS ; macOS →
`wails build` → `.dmg` ; Linux → `wails build` (GTK3 + WebKit2GTK) → `tar.gz`. Les builds
Mac/Linux ne se **cross-compilent pas** depuis Windows (d'où la CI, ou une machine par OS).

## Architecture

```
internal/core     noyau métier (aucune dépendance UI) — appelé à l'identique par la GUI et la CLI
internal/forgejo  client API REST Forgejo
internal/gitx     pilotage du git système
internal/keyring  trousseau local chiffré par le PIN (tokens) — Argon2id + AES-256-GCM
internal/vault    ancien coffre natif OS — conservé uniquement pour la migration v1.0→v1.1.0
internal/ipc      transport local démon ⇄ clients CLI (socket AF_UNIX)
internal/daemon   serveur résident (session, dispatch, modale PIN)
internal/config   configuration locale (aucun secret)
cli/              frontal ligne de commande (Cobra) — client du démon
app.go            frontal fenêtre (Wails) — porte le démon
frontend/dist     interface (HTML/CSS/JS statique, embarquée)
```

## Projets, dépôts, vue « distant ⇄ local »

La page **Projets** s'ouvre sur un **accueil « classeurs »** instantané (aucun appel réseau : identité,
topologie et état du clone **local** lus sur le disque). Un clic ouvre le **détail** d'un projet, qui
vérifie alors en ligne l'alignement **Local · Atelier · Vitrine** (ce qui est sur le poste, dans
l'atelier de dev, en vitrine de production) et offre un **panneau git** : branches (basculer, fusionner,
supprimer — avec alerte si la branche n'a jamais été fusionnée) et **copies de travail** (clones et
*worktrees* : plusieurs branches en parallèle depuis un seul dépôt). La vitrine ne se met à jour que par
**promotion** depuis la fenêtre, sous PIN — jamais poussée depuis le local.

L'explorateur **Dépôts** détaille, pour chaque dépôt, son état **distant** (branche par défaut,
versions/tags, commits commentés, navigation fichiers) et son clone **local** rattaché (branche de
travail, version décrite, avance/retard, modifs non commitées, commits à pousser). Rattachement
automatique par l'URL d'origine, rafraîchissement en direct, filtres par recherche et organisation.

## Historique des versions

Le détail des versions livrées figure dans [`CHANGELOG.md`](CHANGELOG.md).

## Licence

SituLab est un logiciel libre distribué sous la **licence Apache 2.0** — voir [`LICENSE`](LICENSE).
L'avis de droits et de marques figure dans [`NOTICE`](NOTICE).

« SituLab » et le logo SituLab sont des marques d'APCOM Solutions SA et ne sont **pas** couverts
par la licence Apache (section 6, « Trademarks »).

---

© 2026 APCOM Solutions SA — sous licence Apache 2.0.
