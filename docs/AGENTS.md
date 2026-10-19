# SituLab — carte pour agents IA (et démarrage humain)

> Court par choix. **SituLab EST la carte** : interroge-le (`situlab map`, `--output json`,
> `situlab <groupe> --help`) au lieu de deviner. Ce fichier **cadre et oriente** ; il ne recopie pas la
> référence des commandes (elle vit dans `situlab --help`, toujours à jour). Lisible aussi dans l'app
> (menu *Aide*). Binaire : `situlab` (Windows : `situlab.exe`). Version : **1.16.2**.

## Si tu es l'assistant IA — commence ici

1. **SituLab est la porte unique** vers le **git** (Forgejo/GitHub), les **clouds souverains** (CORPUS)
   et les **sauvegardes** (BACKUP) de l'utilisateur. Tu passes **toujours** par lui ; tu n'as **jamais**
   ses jetons ni ses clés.
2. **Interroge, ne devine pas.**
   - `situlab map --output json` = l'organisation du poste (KERNEL, spokes, tags, liens
     local↔atelier↔vitrine).
   - `situlab <groupe> --help` = la **liste exacte** des commandes d'un domaine
     (`repo`·`local`·`kernel`·`spoke`·`cloud`·`backup`·`project`·`issue`·`pull-request`·`release`·`admin`).
     Ne l'apprends pas par cœur ici — demande-la.
3. **Ouverture KERNEL — propose-la d'emblée.** `situlab kernel list` montre les KERNEL et l'actif
   (`*`). S'il y en a un : **propose à l'humain de l'ouvrir** — `situlab kernel open [nom]` donne le
   **fichier d'ouverture** (chemin absolu) → lis-le → suis-le. `situlab kernel pull` d'abord si tu veux
   les dernières notes (sans PIN). Plusieurs KERNEL, ou aucun actif : **demande lequel ouvrir** (lui
   seul choisit l'actif). Même logique pour un spoke (`situlab spoke open <nom>`).
4. **Le PIN n'est pas à toi, et il n'est pas absolu.** Prépare l'action et **lance-la** : **SituLab
   décide** s'il faut une validation (selon la politique de la connexion) et te répond — **fait**,
   **refusé**, ou **délai écoulé** (relance). Une fenêtre s'ouvre sur le poste de l'humain ; **lui seul**
   tape le PIN. Ne le demande jamais, ne le présume jamais.
5. **Signature automatique.** Chaque commit / promotion que tu fais via SituLab est **signé par le
   service de garantie APCOM** (preuve « fait via SituLab », clé jamais sur le poste). Tu n'as rien à
   faire. **Ne retire ni n'altère jamais le trailer `SituLab-Approved:`** — cela efface la preuve.
6. **Notre collaboration** : la section en bas (ajoutée au dépôt) donne la façon de travailler convenue
   avec l'utilisateur. Lis-la ; tu peux l'enrichir. L'humain l'édite aussi dans *Réglages → Assistant IA*.

## La carte — où est quoi

| Tu veux… | Demande à SituLab |
|---|---|
| l'organisation du poste | `situlab map --output json` |
| la liste exacte des commandes d'un domaine | `situlab <groupe> --help` |
| lister les KERNEL (actif marqué `*`) | `situlab kernel list` |
| ouvrir un KERNEL / un spoke | `situlab kernel open` · `situlab spoke open <nom>` |
| l'état (verrou, PIN, instances, git) | `situlab status` · `situlab doctor` |
| ce que tu as le droit de voir dans un cloud | `situlab cloud allowed <cloud>` — **à lire en premier** |
| l'état d'origine des commits (sceau) | `situlab attest status <dossier>` |
| piloter sans terminal (Cursor, Claude Desktop…) | serveur MCP : `situlab mcp` — mêmes capacités, mêmes garanties |

## Les notions (le strict nécessaire)

- **Trois accès, un noyau.** Fenêtre (humain) · CLI `situlab` (agent en shell) · MCP `situlab mcp`
  (agent sans terminal). Mêmes opérations, même consentement PIN, même signature.
- **Les quatre univers.** **KERNEL** = le dépôt qui *est* la mémoire. **SPOKES** = projections de
  mission rattachées à un KERNEL. **CORPUS** = clouds de fichiers souverains, **accès IA borné par
  dossier** (tu ne vois que l'autorisé — fail-closed). **BACKUP** = plans de sauvegarde git → cloud.
- **local ↔ atelier ↔ vitrine.** Le clone **local** a pour `origin` l'**atelier** (lien **EXACT**, même
  URL). La **vitrine** (production) est un dépôt **séparé**, alimenté par **promotion** (version/tag)
  sous PIN — **jamais** par un push direct. Le local ne « connaît » que son atelier.
- **Mémoire situative.** La méthode (un KERNEL vivant, tenu cohérent à chaque geste). Source et
  manifeste : **manifeste.apcom.app**.

## Repères agent

- **Sans argument**, `situlab` ouvre la fenêtre (humain) : un agent ne la lance jamais. **Avec une
  commande**, c'est le mode agent — ajoute toujours `--output json`.
- Un **démon résident** porte la session déverrouillée ; chaque invocation CLI est un process neuf qui
  lui parle. Lecture = session déverrouillée ; écriture distante = PIN confirmé dans la fenêtre.
- **Codes de sortie** : `0` succès · `1` erreur générique · `2` instance introuvable · `3` PIN requis /
  session verrouillée en non-interactif · `4` git absent.
- Joins `--intent "pourquoi"` (CLI) ou l'argument `intent` (MCP) à une mutation : la fenêtre PIN
  l'affiche **à côté** de ce que SituLab **constate** (déclaratif ; seul le PIN autorise).
- **Mode lecture seule** (réglé en fenêtre) : refuse toute mutation — git **et** cloud — même pour toi.

## Configuration (où vivent les données)

`%AppData%\SituLab\` (Windows) · `~/.config/SituLab/` (Linux) ·
`~/Library/Application Support/SituLab/` (macOS) : `config.json` (aucun secret) +
`keyring.json` (jetons **chiffrés par le PIN** : Argon2id → KEK → DEK, AES-256-GCM). Le PIN n'est
stocké nulle part — il *est* la clé.

## Démarrage humain (30 secondes)

Lance SituLab, choisis un **PIN**, ajoute une connexion (nom + URL Forgejo + token, ou GitHub + PAT).
L'écran **Dépôts** se tient à jour tout seul ; un clic ouvre le détail (Aperçu / Fichiers / Historique).
Rien ne part en ligne sans ton **PIN**.

---
<!-- Notre collaboration — section ajoutée au dépôt, éditable dans Réglages → Assistant IA.
     L'agent la lit au démarrage et peut l'enrichir (préférences, conventions, dépôts clés). -->
