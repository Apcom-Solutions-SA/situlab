# Journal des modifications — SituLab

Format : [Keep a Changelog](https://keepachangelog.com/fr/). Versions : [SemVer](https://semver.org/lang/fr/).

**La 1.19.0 est la première version publique de production de SituLab.** Les versions antérieures
étaient des itérations internes, hors production : elles ne sont ni distribuées ni documentées ici.
L'historique public et le développement communautaire commencent à la 1.19.

## [1.19.0]

Première version publique. Cette entrée présente SituLab dans son état 1.19 : ses fonctions, ses
modules et ses principes de sécurité.

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
