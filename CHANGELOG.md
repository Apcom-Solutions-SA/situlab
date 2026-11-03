# Journal des modifications — SituLab

Format : [Keep a Changelog](https://keepachangelog.com/fr/). Versions : [SemVer](https://semver.org/lang/fr/).

## [1.21.0] — 2026-11-03

Première version publique de SituLab. Cette entrée présente le logiciel dans son état 1.21 : ses
fonctions, ses modules et ses principes de sécurité.

### Ce qu'est SituLab

L'utilitaire git et mémoire pour tous : une application de bureau multiplateforme qui rend lisible ce
que vous avez en ligne et en local, sans navigateur ni maîtrise fine de git, et qui tient votre mémoire
de travail en ordre, pour vous comme pour vos agents IA. Le principe : un agent prépare, l'humain
débloque par son PIN.

### Trois accès, un seul noyau

- Fenêtre graphique (humain) : application de bureau native.
- Ligne de commande `situlab` (agents en shell).
- Serveur MCP `situlab mcp` (agents sans terminal, comme Cursor ou Claude Desktop).

Un seul binaire : sans argument il ouvre la fenêtre, avec une commande il agit en mode agent. Parité
entre les trois accès, même point de consentement par PIN. Le classement et la désignation (projets,
KERNEL, spokes, tags, droits) restent réservés à la fenêtre.

### Les modules

- **Dépôts git** : appariement du clone local, de l'atelier et de la vitrine ; état « poste, atelier,
  production » en un coup d'œil ; classement par tags ; scan d'un dossier pour suivre vos dépôts en lot.
- **KERNEL** : le dépôt qui porte votre mémoire, désigné, lisible, synchronisé en deux gestes.
- **SPOKES** : les projections de mission rattachées à un KERNEL.
- **CORPUS** : vos clouds souverains (Nextcloud, kDrive), avec un accès IA borné dossier par dossier.
- **BACKUP** : vos sauvegardes des dépôts vers un cloud, sans PIN, à l'ouverture, à la fermeture ou à la
  demande.
- **Agent IA** : brancher un assistant (terminal ou MCP) sur une fiche de collaboration, sous votre PIN.

### Confort et exploitation

- **Rafraîchissement automatique de l'état** : après une opération git (en ligne de commande, par un
  agent ou dans la fenêtre), la vue se met à jour d'elle-même, sans attendre la récupération périodique ;
  une icône de rafraîchissement manuel complète le détail d'un projet.
- **Journal local des opérations** : une trace locale (qui reste sur le poste, n'est jamais envoyée et ne
  contient aucun secret) de ce que fait SituLab : surface, succès ou échec, erreur, durée. Consultable
  dans **Réglages → Journaux**, en ligne de commande (`situlab journal tail|level|clear`) et par un agent
  via l'outil MCP `journal_read` (lecture seule). Trois niveaux : désactivé, échecs seulement, tout.

### Git

Opérations locales en git natif (statut, branches, commits, merge, rebase, cherry-pick, stash,
worktrees). Tout échange réseau (clone, fetch, pull, push, synchronisation) passe par le démon, sous PIN
selon le rôle de la connexion. Promotion d'une version validée de l'atelier vers la production, toujours
sous PIN, jamais par un push direct.

### Fournisseurs

Forgejo, Gitea et GitHub. Pour Forgejo, la première connexion crée elle-même un jeton à partir de vos
identifiants, ou vous collez le jeton que vous avez créé ; pour GitHub, vous collez un jeton d'accès
personnel. Administration, statistiques et gestion de jetons sont propres à Forgejo.

### Sécurité (principes)

- Jetons chiffrés au repos : trousseau local, clé dérivée du PIN (Argon2id puis AES-256-GCM). Le PIN
  n'est stocké nulle part ; il est la clé.
- Le PIN ne transite jamais par le socket ni par l'agent : il se saisit dans une fenêtre sur le poste.
- Le jeton n'est jamais remis au client : c'est le démon qui appelle l'API.
- git durci : hooks, assistant d'identifiants, transports ext et file, invites neutralisés ; une URL en
  `http://` est refusée pour un hôte public.
- Droits cloud par chemin, fermés par défaut : l'agent ne voit que ce qui est autorisé.
- Mode lecture seule et verrou après écriture.
- Export de configuration chiffré (`.slbak`), sans aucun secret en clair.
- Signature « SituLab approved » : attestation d'origine des commits, vérifiable localement.

Modèle de sécurité complet et limites assumées : `SECURITY.md`.

### Qualité (pour les contributeurs)

Intégration continue sur une matrice de trois systèmes (Linux, Windows, macOS). Portes bloquantes :
format (`gofmt`), analyse statique (`go vet`), style (`golangci-lint`, jeu standard), tests avec
détecteur de courses (`go test -race`), vulnérabilités des dépendances (`govulncheck`), et style du
frontend (ESLint). Code organisé par domaine (fichiers courts). Décisions d'architecture tracées dans
`docs/DECISIONS/`. Flux de contribution et portes de qualité : `CONTRIBUTING.md`.
