# SituLab 1.1.0 — Recette de contrôle (tous cas de figure)

> Révision sécurité (trousseau chiffré par PIN, démon + modale, onboarding PIN-only).
> Légende : **[auto]** couvert par un test Go (`go test ./...`) · **[build]** vérifié à la
> compilation / `node --check` · **[écran]** à vérifier visuellement en session bureau (non
> automatisable ici) · **[réel]** nécessite un vrai compte Forgejo.

## 0. Pré-requis de build
- [auto/build] Toolchain Go **1.23.4** (`GOTOOLCHAIN=go1.23.4`) ; `go vet ./...`, `go test ./...`,
  `gofmt -l` vides ; `node --check frontend/dist/main.js` OK.
- [build] Binaire console : `go build -tags "desktop,production" -ldflags "-X main.version=1.1.0 -s -w"`
  → `situlab.exe` ; `situlab version` répond **sans** démarrer le démon (connexion paresseuse).

## 1. Trousseau chiffré (internal/keyring)
- [auto] Init → Unlock : la DEK déballée est identique (roundtrip).
- [auto] Mauvais PIN → `ErrBadPIN` (le tag GCM échoue).
- [auto] set/get token, delete idempotent, `Has`.
- [auto] ChangePIN : l'ancien PIN ne déverrouille plus, le nouveau oui, les tokens restent lisibles.
- [auto] Fichier non initialisé → `ErrNotInitialized` ; déjà initialisé → `ErrAlreadyInit`.
- [auto] Entrée altérée (flip d'octet) détectée → `ErrFormat`.
- [auto] **Paramètres KDF hors bornes (p=0, …) → `ErrFormat`** (pas de panique argon2 / DoS démon).
- [auto] **Version future (≥2) → `ErrFormat`** (pas de réécriture silencieuse en v1).
- [écran] Déverrouillage < ~300 ms sur le poste (mesuré ~60 ms).

## 2. Session & PIN (internal/core)
- [auto] Définir le PIN ouvre la session ; `Lock` la ferme ; token lisible seulement déverrouillé.
- [auto] Re-`Unlock` rouvre ; mauvais PIN à l'`Unlock` échoue.
- [auto] **Anti-bruteforce en mémoire non écrasé par une écriture de config concurrente** : le
  verrou 5 essais / 60 s tient malgré des `SetDefaultInstance` interleavés.
- [auto] `SetPIN` (changement) rate-limité par le même verrou que Push (régression v0.3 conservée).
- [écran] Re-verrouillage automatique après 15 min d'inactivité (la DEK est zéroisée par la minuterie).

## 3. Migration coffre OS → trousseau (upgrade v1.0 → v1.1.0)
- [auto] Poste v1.0 (instance + token au coffre OS simulé) → définir le PIN → token importé dans le
  trousseau + **coffre OS purgé** + `Doctor().VaultResidual == 0`.
- [code] Migration **retentée** à chaque ouverture tant que non entièrement réussie ; purge toujours
  retentée (plus de court-circuit `kr.Has` qui bloquait une purge ratée).
- [réel] Sur un vrai poste avec Windows Credential Manager peuplé.

## 4. Démon & IPC
- [auto] Transport AF_UNIX : roundtrip, multi-appels/connexion, réponse d'erreur typée, démon absent,
  charge volumineuse. [auto] `ipc.Listen` refuse (ErrAlreadyRunning) si un démon vivant écoute.
- [auto] Dispatch : status / pin.set / lock ; unlock via modale ; annulation → `ErrLocked` ;
  writePIN via modale (ou refus → `ErrPINNeeded`) ; opération inconnue → erreur.
- [auto] Chaîne complète client CLI → IPC → démon → noyau (SetPIN/PINSet/Instances/Doctor + erreur typée).
- [écran] Auto-démarrage : 1re commande CLI lance la fenêtre+démon, puis les suivantes s'y connectent.

## 5. Modèle de consentement (le PIN ne transite jamais par le socket)
- [code] `writePIN` et `OpUnlock` passent **uniquement** par la modale ; champ `Args.PIN` et flags
  `--pin` retirés. Un agent connaissant le PIN ne peut plus autoriser une écriture sans la fenêtre.
- [écran/réel] Agent lance `situlab dashboard` (session verrouillée) → **la fenêtre PIN surgit sur le
  bureau** ; l'agent reçoit le résultat après saisie, ou un refus (code 3) si Refuser/Échap.
- [écran] `situlab push <dir>` → modale « Autoriser l'envoi… » ; Échap/Refuser → code 3 immédiat
  (plus de gel de 2 min).

## 6. Onboarding PIN-only (fenêtre + CLI)
- [build] GUI : PIN (création+confirmation) → identifiant + mot de passe → 2FA **progressive**
  (révélée au 1er échec) → SituLab crée le token et le chiffre.
- [code] `AddInstanceLogin` : doublon vérifié **avant** la création du token (pas d'orphelin) ;
  token serveur **révoqué** si la persistance échoue ; nom `SituLab@<poste>/<instance>` (anti-collision).
- [code] CLI `instance login` : réessai 2FA proposé **seulement** sur une erreur d'auth (pas doublon/URL/verrou).
- [écran/réel] Compte **sans** 2FA : le champ 2FA n'apparaît jamais. Compte **avec** 2FA : il apparaît
  au bon moment ; mot de passe et OTP jamais stockés.

## 7. Retour dans l'app & déverrouillage GUI
- [écran] Rouvrir l'app configurée → **écran de déverrouillage** (PIN) → tableau de bord (plus
  d'« app inutilisable » / bouton Réessayer sans fin).
- [écran] Re-verrouillage en cours d'usage (15 min) → l'écran propose de re-déverrouiller.
- [écran] Réglages : état de session affiché, boutons Verrouiller/Déverrouiller.

## 8. Gestion des tokens & utilisateurs (fenêtre)
- [écran] Onglet Tokens : distinction claire « jetons Forgejo » vs accès interne `SituLab@…` ;
  **ce dernier n'a pas de bouton supprimer** ; portées en langage clair.
- [écran] Création de token : validations (nom requis, dépôts spécifiques ⇒ ≥1 coché), valeur
  affichée **une seule fois** avec bouton Copier.
- [écran] Administration : masquée en douceur si non-admin ; pastilles Administrateur/Standard ;
  création d'utilisateur avec aides + récapitulatif.
- [réel] Gestion de tokens sur compte 2FA : limitation connue (l'auth basique des endpoints de
  tokens exige l'OTP ; champ 2FA du gestionnaire non encore branché — voir §11).

## 9. Lectures & boucle git (selon droits du token)
- [écran/réel] whoami, dépôts, branches/tags/commits/arbre/fichier, orgs ; dashboard distant⇄local vivant.
- [écran/réel] clone / fetch / pull / commit / diff ; la vue **diff n'est plus écrasée** par le live.
- [auto] Codes de sortie CLI stables : 0 / 1 / 2 (instance) / 3 (PIN/verrou) / 4 (git absent).

## 10. Robustesse
- [auto] Config corrompue → `.bak` + repart ; trousseau corrompu → `ErrFormat`.
- [écran] Double-clic sur « Valider » d'une modale → une seule écriture (bouton désactivé pendant l'appel).
- [écran] `doctor` sur poste sans fenêtre possible (headless) → remonte l'erreur (plus de rapport
  faux + code 0).

## 11. Limites connues (assumées pour 1.1.0, à traiter plus tard)
- **Headless/CI** : le modèle « PIN via modale sur le bureau » rend la CLI inutilisable sans session
  graphique (pas de modale → pas de déverrouillage/écriture). À adresser par un mode dédié ultérieur.
- **Gestion des tokens sur compte 2FA** : champ OTP du gestionnaire (list/create/delete) non branché.
- **Course inter-process** (2 fenêtres SituLab en parallèle) sur le trousseau : écriture
  « dernier qui gagne » ; verrou de fichier inter-process à ajouter (déjà en feuille de route).

> Nettoyage fait en 1.1.0 : le paquet `internal/pin` (ancien condensé de consentement) a été
> **supprimé** — le consentement passe désormais par le déballage du trousseau.

---

© 2026 APCOM Solutions SA.
