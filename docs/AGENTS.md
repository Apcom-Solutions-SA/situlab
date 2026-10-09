# SituLab — Guide d'utilisation (humains & agents IA)

> Un seul document, deux lectures. Les humains trouveront en tête un démarrage rapide ; les agents
> trouveront ensuite la référence complète de la ligne de commande. Il est aussi lisible **dans
> l'application** (menu *Aide*). Binaire : `situlab` (Windows : `situlab.exe`). Version : **1.9.0**.

## Pour ton assistant IA — commence ici

Si tu es l'assistant IA de l'utilisateur : ce fichier est à la fois **ton mode d'emploi** de SituLab
**et** votre **fiche de collaboration**. Au démarrage d'une session de travail :
1. lis ce guide pour savoir piloter SituLab (CLI `situlab`, ou outils MCP `repo_*`/`issue_*`/`git_*`) ;
2. lis la section **« Notre collaboration »** en bas (ajoutée au moment du dépôt) : tu y trouveras
   la façon de travailler convenue avec ton utilisateur ;
3. **règle d'or** : tu prépares, l'humain débloque par son PIN. Ne demande jamais le PIN, ne cherche
   pas à le contourner ; rien ne part en ligne sans sa validation.

Tu **peux enrichir** la section « Notre collaboration » de ce fichier au fil de l'eau (préférences,
conventions, dépôts clés). L'utilisateur peut aussi l'éditer dans SituLab (Réglages → Assistant IA).

## Pour un humain — en 30 secondes

SituLab montre, pour chacun de vos dépôts Forgejo, **ce que vous avez en ligne et en local, et dans
quel état** — sans ouvrir de navigateur.

1. Lancez SituLab (double-clic). À la première ouverture, choisissez un **PIN**, puis saisissez le
   **nom**, l'**URL** de votre instance (ex. `https://git.apcom.app`) et votre **token personnel**.
   Le token est **chiffré par votre PIN** sur le poste ; ensuite, seul le PIN vous est redemandé.
2. L'écran **Dépôts** se remplit et se tient à jour tout seul. Chaque carte montre le **distant**
   (branche, versions) et votre **clone local** rattaché (branche de travail, avance/retard, modifs).
3. Cliquez une carte pour le détail : **Aperçu** (distant vs local), **Fichiers** (naviguer, lire),
   **Historique** (versions et commits).

Rien à pousser sans votre **PIN** : les actions qui écrivent vers le distant le demandent.

## Pour un agent — principes

- **Sans argument**, `situlab` ouvre la fenêtre (usage humain) : un agent ne la lance jamais.
- **Avec une commande**, c'est le mode agent. Ajoutez toujours `--output json`.
- **Un démon résident porte la session.** La 1re commande démarre SituLab (fenêtre + démon) si
  besoin ; les commandes suivantes lui parlent. La session déverrouillée vit dans le démon, pas
  dans la commande (chaque invocation est un process neuf).
- **Le PIN ne passe jamais par l'agent.** Quand une opération l'exige — déverrouiller pour lire,
  confirmer pour écrire — une **fenêtre de saisie surgit sur le poste de l'humain** ; lui seul tape
  le PIN. L'agent reçoit « fait » ou « refusé », jamais le PIN.
- **Lecture = session déverrouillée. Écriture distante = PIN confirmé à chaque fois.** En
  non-interactif (aucun humain pour la fenêtre), l'action échoue avec le code **3** — c'est voulu.
- Les **valeurs de tokens ne sont jamais exposées**. Les tokens vivent **chiffrés par le PIN**
  (plus de coffre OS).

## Codes de sortie

| Code | Sens |
|---|---|
| 0 | Succès |
| 1 | Erreur générique (arguments, réseau, API) |
| 2 | Instance introuvable / non configurée |
| 3 | PIN requis / session verrouillée en mode non-interactif (push, ou lecture sans session) |
| 4 | git absent de la machine |

## Configuration

`config.json` (instances, compteur anti-bruteforce du PIN, dépôts locaux suivis) et
`keyring.json` (tokens **chiffrés par le PIN** : Argon2id → KEK → DEK, AES-256-GCM) dans :
`%AppData%\SituLab\` (Windows) · `~/.config/SituLab/` (Linux) ·
`~/Library/Application Support/SituLab/` (macOS). **Aucun secret en clair** : le PIN n'est stocké
nulle part (il *est* la clé), et il n'y a **plus de coffre OS** (migré automatiquement au 1er
déverrouillage depuis une version antérieure).

## Référence des commandes

Indicatifs globaux : `--output text|json` (défaut `text`), `--instance <nom>` (défaut : instance par défaut).

### Système
- `situlab version` — version.
- `situlab doctor` — git présent ? config ? trousseau ? PIN ? session ? résidu de coffre OS ?

### Session (démon)
- `situlab status` — session verrouillée/déverrouillée, PIN défini, nombre d'instances.
- `situlab unlock` — déverrouille la session : le démon ouvre la **fenêtre de PIN** sur le poste
  (le PIN ne transite jamais par la CLI).
- `situlab lock` — verrouille la session (oublie la clé en mémoire).
- `situlab purge --confirm` — **efface TOUTES les données SituLab de ce poste** (connexions,
  trousseau chiffré, réglages, jetons du coffre Windows). IRRÉVERSIBLE. Tourne hors de la fenêtre
  (refuse si la fenêtre SituLab est ouverte — ferme-la d'abord). Pour repartir à blanc ou céder le
  poste. (Aussi disponible dans la fenêtre : Réglages → Sécurité → « Effacer mes données ».)

### Instances
- `situlab instance add <nom> --url <url> --token <token>` — ajoute une instance **Forgejo/Gitea**
  et **chiffre le token par le PIN** (session déverrouillée requise ; à défaut, le démon la demande
  par la fenêtre de PIN).
- `situlab instance add <nom> --provider github --token <PAT>` — ajoute un compte **GitHub** (jeton
  d'accès personnel collé ; l'URL est fixée à github.com). GitHub ne permet pas la gestion des
  comptes ni des jetons via l'API : les onglets Administration, Statistiques et Jetons sont propres
  à Forgejo.
- `situlab instance list` · `instance test <nom>` · `instance remove <nom>` · `instance default <nom>`.

### Vue unifiée (le cœur)
- `situlab dashboard` — par dépôt : distant (branche par défaut) + local rattaché (version décrite,
  branche, avance/retard, état). Rattachement par l'URL d'origine.

### Lecture distante (selon droits du token)
- `situlab whoami`
- `situlab repo list`
- `situlab repo view <owner>/<repo>`
- `situlab repo branches <owner>/<repo>`
- `situlab repo tags <owner>/<repo>`
- `situlab repo commits <owner>/<repo> [--ref <b>] [--limit n]`
- `situlab repo tree <owner>/<repo> [chemin] [--ref <b>]`
- `situlab repo file <owner>/<repo> <chemin> [--ref <b>]`
- `situlab org list`
- `situlab token list` — noms + portées (jamais les valeurs).

### Local (git)
- `situlab local add <chemin>` · `local list` · `local status <chemin>`
- `situlab repo clone <owner>/<repo> <dossier>`

### Boucle de travail locale (v1)
- `situlab repo fetch <dossier>` — vérifie les nouveautés du serveur (sans PIN).
- `situlab repo pull <dossier>` — récupère en fast-forward (sans PIN).
- `situlab repo commit <dossier> --message "…"` — enregistre un point local (sans PIN).
- `situlab repo diff <dossier>` — modifications non enregistrées.
- `situlab repo clone <owner>/<repo> <dossier>` — copie un dépôt sur le poste.
- `situlab local remove <chemin>` — cesse de suivre un dépôt local.
- `situlab local branch <chemin> <nom>` / `local switch <chemin> <branche>` — créer / changer de branche.
- `situlab local log <chemin> [--limit n]` · `local stash-save|stash-list|stash-pop <chemin>`.
- `situlab local discard <chemin> [--file <f>] --confirm` — annule les modifs locales (DESTRUCTIF).

### Git « complet » — gestes de dev avancés (v1.6, CLI/MCP ; locaux = sans PIN)
Ces gestes **ne touchent pas au distant** → **aucun PIN**. En cas de conflit, la commande renvoie
un message guidé stable (« des fichiers sont en conflit… ») **sans jamais enregistrer de marques** :
résous les fichiers, puis `…-continue` (ou `…-abort` pour renoncer). Réservés à la CLI et au MCP
(l'IA travaille là) ; la fenêtre reste volontairement au niveau simple.
- **Réunir des branches** : `situlab local merge <chemin> <ref>` (+ `local merge-abort <chemin>`) ·
  `situlab local rebase <chemin> <upstream>` (+ `local rebase-continue|rebase-abort <chemin>`).
- **Nettoyer l'historique** : `situlab local cherry-pick <chemin> <commit>`
  (+ `cherry-pick-continue|cherry-pick-abort <chemin>`) · `situlab local revert <chemin> <commit>`
  (non destructif) · `situlab local reset <chemin> <ref> [--mode soft|mixed|hard]`
  (`--mode hard` est destructif → exige `--confirm`) · `situlab local amend <chemin> [--message …]`
  (réécrit le dernier commit — uniquement s'il n'est pas publié).
- **Commits soignés** : `situlab local commit-paths <chemin> --message "…" --path a --path b`
  (staging **sélectif** : n'enregistre que les chemins listés, évite les fichiers parasites).
- **Tag local** : `situlab local tag-create <chemin> <nom> [--message …]` (publier ensuite avec
  `push-tag`).
- **Pousser après réécriture (distant → PIN)** : `situlab local push-force <chemin>`
  (`--force-with-lease --force-if-includes` : « force » **sûr**, refuse d'écraser un travail
  distant apparu entre-temps — y compris récupéré par l'auto-fetch de fond)
  · `situlab local push-tag <chemin> <tag>`.

> Les marques de conflit ne sont **jamais** enregistrées : `commit`, `amend` et `commit-paths`
> refusent (message guidé) si des fichiers en index en contiennent — résous d'abord.
> Le garde-fou `--confirm` du `reset --mode hard` est propre à la **CLI** (confort opérateur) ;
> en MCP, comme `git_discard`, les gestes destructifs **locaux** reposent sur le mode lecture seule
> et `assertWritable`, pas sur une confirmation supplémentaire (le PIN protège le **distant**).

### Rôles, politique PIN & projets (v1.7)

Chaque connexion a un **rôle** : **atelier** (terrain de dev) ou **production** (vitrine). Le rôle
et la **politique PIN par connexion** (par action : envoi / issues / admin dépôt / admin comptes)
se règlent **dans la fenêtre uniquement** — un agent ne peut pas les changer. Défauts : **atelier**
autonome (envoi & issues sans PIN sur session déverrouillée, admin sous PIN) ; **production** tout
sous PIN. L'exigence de PIN d'un envoi suit le **dépôt réellement poussé** (son serveur), pas la
connexion que tu nommes : tu ne peux donc pas pousser en production via une connexion atelier.

Rôle et politique sont lisibles dans `instance list` / `dashboard` (`role`). Classement à respecter :
**tu travailles librement en atelier, tu prépares la production mais tu ne la publies jamais sans
le PIN de l'humain.**

- **Projets** (lien atelier↔production, lecture) : `situlab project list` ·
  `situlab project status <nom>` (équilibre : version atelier vs prod, « équilibré » / « en retard de
  N versions »). Les liens se créent dans la fenêtre.
- **Promotion** : `situlab project promote <nom> --prod <index>` publie la version de l'atelier vers
  une production, en **une version propre** (pas les micro-commits). Vers la production → **PIN**
  confirmé dans la fenêtre. Jamais de jeton remis à l'agent.

### Écriture distante — PIN requis
- Le PIN se **définit dans la fenêtre** (Réglages → Sécurité) — jamais par la CLI (`situlab pin set`
  ne fait qu'y renvoyer, pour que le PIN ne circule pas sur le socket).
- `situlab repo push <dossier>` — pousse ; exige le PIN (saisi dans la fenêtre).
- `situlab repo sync <dossier> [--message …]` — **synchronise en un geste** : récupère les
  nouveautés (avance rapide), enregistre **toutes** les modifications en cours (fichiers nouveaux
  compris — attention aux fichiers sensibles non ignorés par `.gitignore`), puis envoie ; exige le
  PIN. En cas de changements croisés, renvoie une erreur claire (« à réunir ») plutôt qu'une sortie
  git brute.
- `situlab repo create <nom> [--description …] [--private] [--org <organisation>]` — crée un dépôt.
  Sans `--org`, dans l'espace personnel ; beaucoup de serveurs l'interdisent → utiliser `--org`.
- `situlab repo delete <owner>/<repo>` — supprime définitivement un dépôt (PIN demandé sur le poste).
- `situlab repo setdesc <owner>/<repo> --desc "…"` — modifie la description.
- `situlab issue list <owner>/<repo> [--state open|closed|all]` — lecture.
- `situlab issue create <owner>/<repo> --title "…" [--body "…"]` — crée une issue.

Les écritures déclenchent une demande de PIN **sur le poste** (fenêtre de SituLab) ; le PIN ne
transite jamais par la CLI ni par l'agent.

### Tokens (auth basique : identifiant + mot de passe du compte, non stockés)
- `situlab token list` — tes tokens (noms + portées).
- `situlab token create --user <u> --password <p> --name <n> [--scopes a,b] [--repos owner/repo,…]`
  — crée un token ; `--repos` le restreint à ces dépôts.
- `situlab token delete <id> --user <u> --password <p>`.

### Administration (token administrateur)
- `situlab admin users` — liste les utilisateurs.
- `situlab admin directory` — **annuaire consolidé** : les comptes de toutes les instances Forgejo
  réunis, avec la présence par instance (actif/suspendu/inactif/absent). Lecture seule, tolérant aux
  instances injoignables.
- `situlab admin user-create <identifiant> --email … --password … [--admin]`.

> **Mode lecture seule** (v1.4) : activable dans la fenêtre (Réglages → Sécurité). Quand il est
> actif, toute mutation (envoi, synchronisation, suppression, administration, annulation locale)
> est refusée avec « mode lecture seule actif » — y compris pour les agents CLI/MCP. Seule la
> fenêtre peut le désactiver.

### Issues (écritures — PIN sur le poste)
- `situlab issue comment <owner>/<repo> <n> --body "…"` · `issue close|reopen <owner>/<repo> <n>` · `issue edit <owner>/<repo> <n> [--title …] [--body …]`
- `situlab issue labels <owner>/<repo> <n> --label a --label b` (remplace par noms) · `issue label-list <owner>/<repo>`
- `situlab issue assign <owner>/<repo> <n> --user alice --user bob`
- `situlab issue milestone <owner>/<repo> <n> --id <milestoneID|0>` · `issue milestone-list <owner>/<repo>`

### Demandes de fusion (lecture + écritures — PIN)
- `situlab pull-request list <owner>/<repo> [--state open|closed|all]`
- `situlab pull-request detail|diff|files <owner>/<repo> <n>` (lecture)
- `situlab pull-request create <owner>/<repo> --title "…" --head <branche> --base <branche> [--body "…"]`
- `situlab pull-request review <owner>/<repo> <n> --event APPROVED|REQUEST_CHANGES|COMMENT [--body "…"]`
- `situlab pull-request merge <owner>/<repo> <n> [--method merge|rebase|squash]` · `pull-request close <owner>/<repo> <n>`

### Versions & tags (écritures — PIN)
- `situlab release create <owner>/<repo> <tag> [--name …] [--body …] [--target <ref>] [--prerelease]`
- `situlab release delete <owner>/<repo> <id>` · `release tag-create <owner>/<repo> <tag> [--target <ref>]` · `release tag-delete <owner>/<repo> <tag>`

### Réglages de dépôt (écritures — PIN)
- `situlab repo rename <owner>/<repo> <nouveau-nom>` · `repo visibility <owner>/<repo> --private=true|false`
- `situlab repo archive <owner>/<repo> --archived=true|false` · `repo default-branch <owner>/<repo> <branche>`
- `situlab repo delete-branch <owner>/<repo> <branche>` — supprime une branche distante (PIN ; refuse la branche par défaut).

### Signature de garantie « SituLab approved »
- `situlab attest status <dossier>` — état d'attestation des derniers commits d'un dépôt local (✅ par Untel / ⬜). Lecture.
- `situlab attest genkey --instance <nom>` — génère la clé d'appareil et affiche sa clé publique à faire certifier par la gouvernance.
- `situlab attest install-cert <cert.json> --instance <nom>` — installe le certificat d'appareil émis par la gouvernance.
- La signature est **non bloquante** et apposée automatiquement sur les commits créés par SituLab. Limite T2 : voir `SECURITY.md`.

## Accès MCP (agents sans terminal)

SituLab expose un **serveur MCP** (Model Context Protocol) local sur **stdio**, en plus de la
CLI et de la fenêtre. C'est le 3ᵉ frontal, au-dessus du même noyau — mêmes garanties de
sécurité : **lectures libres** (session déverrouillée), **mutations distantes sous PIN** (la
fenêtre de SituLab s'ouvre sur le poste, l'humain confirme ; l'agent ne peut pas contourner).

Lancer : `situlab mcp` (le démon SituLab, la fenêtre, doit tourner).

Déclarer dans un client MCP (Cursor, Claude Desktop, …) :
```json
{ "mcpServers": { "situlab": { "command": "situlab", "args": ["mcp"] } } }
```

Outils exposés (parité CLI) :
- **Lectures** : `whoami`, `instance_list`, `dashboard`, `repo_list`, `repo_view`,
  `repo_branches`, `repo_tags`, `repo_commits`, `repo_tree`, `repo_file`, `org_list`,
  `issue_list`, `pull_list`, `pull_detail`, `pull_diff`, `pull_files`, `label_list`,
  `milestone_list`, `local_status`, `project_list`, `project_status` (projets & équilibre, v1.7).
- **Local (sans PIN)** : `repo_clone`, `repo_fetch`, `repo_pull`, `repo_commit`,
  `git_branch_create`, `git_switch`, `git_log`, `git_discard`, `git_stash_save`,
  `git_stash_list`, `git_stash_pop`.
- **Git complet local (v1.6, sans PIN)** : `git_merge` (+ `git_merge_abort`), `git_rebase`
  (+ `git_rebase_continue`/`git_rebase_abort`), `git_cherry_pick`
  (+ `git_cherry_pick_continue`/`git_cherry_pick_abort`), `git_revert`, `git_reset`
  (mode soft|mixed|hard), `git_amend`, `git_commit_paths` (staging sélectif), `git_tag_create`.
- **Mutations distantes (PIN requis)** : `git_push_force`, `git_push_tag`,
  `repo_push`, `repo_sync`, `repo_create`, `repo_delete`,
  `repo_rename`, `repo_visibility`, `repo_archive`, `repo_default_branch`,
  `issue_create`, `issue_comment`, `issue_close`, `issue_reopen`,
  `issue_set_labels`, `issue_set_assignees`, `issue_set_milestone`,
  `pull_create`, `pull_merge`, `pull_review`, `pull_close`, `pull_edit`,
  `release_create`, `release_delete`, `tag_create`, `tag_delete`, `project_promote` (publication
  atelier→production, v1.7).

Un outil de mutation renvoie une **erreur « PIN requis / refusé »** si l'humain ne confirme pas
— jamais contournable par l'agent. Le jeton n'est jamais remis à l'agent (ni par la CLI, ni par
MCP) : c'est le démon qui agit.

## Exemples agent

```bash
situlab instance add apcom --url https://git.apcom.app --token <TOKEN>
situlab dashboard --output json
situlab repo tree apcom/SituLab --output json
situlab repo file apcom/SituLab README.md
situlab repo push C:/work/SituLab --output json   # échoue en code 3 sans PIN humain
```
