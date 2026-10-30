# SituLab v1 — Architecture de sécurité des credentials

> Spéc d'implémentation. Remplace le stockage des tokens dans le coffre natif de l'OS par un
> **keyring local chiffré dont la seule clé est le PIN utilisateur**. Objectif : qu'aucun token
> (lecture ou écriture) ne soit exploitable sur le poste sans un geste humain explicite.

## 1. Problème résolu

En v0.3, les tokens sont rangés dans le coffre OS (Windows Credential Manager / Keychain /
Secret Service). Ce coffre protège contre un autre utilisateur, un vol de disque à froid et toute
lecture d'un fichier de config — mais **pas** contre un processus tournant déjà sous la session de
l'utilisateur : l'API du coffre (`CredRead`, etc.) rend le secret en clair à l'identité autorisée.
Un agent lancé sous cette session peut donc récupérer le token et agir **sans passer par SituLab**
(`git push` direct), court-circuitant le PIN.

Conséquence : en v0.3, le PIN est un portillon vérifié par SituLab, pas un verrou cryptographique.
La v1 fait du PIN **la clé sans laquelle les tokens n'existent pas en clair**.

## 2. Principes

1. **Aucun token dans le coffre OS.** Tout credential vit dans un keyring applicatif chiffré,
   dans le `data dir` de SituLab sur le poste.
2. **Le PIN est la seule source de la clé.** Le PIN n'est stocké nulle part, ni en clair ni haché
   pour cet usage. Sans PIN, le keyring est une suite d'octets illisible.
3. **PIN unique** pour tout SituLab (tous comptes Forgejo confondus).
4. **Deux niveaux de geste** :
   - *Déverrouiller* (PIN) → autorise les **lectures** pour la durée d'une session.
   - *Confirmer* (PIN) → exigé à **chaque écriture distante** (push), même session déverrouillée.
5. **Résidu assumé** : pendant une session déverrouillée, la clé de données et le token déchiffré
   existent en mémoire ; un agent très intrusif sous la session pourrait les capter à cet instant.
   On ne l'élimine pas en 100 % local ; on réduit un secret *permanent et rappelable à volonté* à
   un secret *présent seulement pendant une fenêtre que l'humain a ouverte*.

## 3. Modèle cryptographique — key-wrapping (DEK / KEK)

Le PIN ne chiffre **pas** chaque token directement. On interpose une clé de données enveloppée :

```
PIN ──Argon2id(salt, params)──▶ KEK (clé de chiffrement de clés, 32 o, en mémoire seule)
DEK  = 32 octets aléatoires (CSPRNG)                 ◀── générée une fois, à l'init du keyring
wrap = AEAD_encrypt(key=KEK, plaintext=DEK)          ◀── la seule forme persistée de la DEK
token_i chiffré = AEAD_encrypt(key=DEK, plaintext=token_i, aad=entry_id)
```

- **KEK** : dérivée du PIN à chaque déverrouillage, jamais persistée.
- **DEK** : clé aléatoire unique du keyring ; chiffre tous les tokens. Persistée **uniquement
  sous forme `wrap`** (enveloppée par la KEK).
- **AEAD** : AES-256-GCM ou XChaCha20-Poly1305 (nonce aléatoire par entrée, jamais réutilisé).
  L'`entry_id` sert d'AAD pour lier le chiffré à son emplacement logique.

**Changement de PIN** → ré-dérive une nouvelle KEK et ré-enveloppe la DEK (`wrap`). Les tokens
eux-mêmes ne sont **pas** re-chiffrés. Rapide et sans risque de perte.

**Dérivation Argon2id** (reprend le durcissement v0.3) : paramètres calibrés poste (p.ex.
`m=64 MiB, t=3, p=1`), `salt` aléatoire de 16 o par installation, stocké en clair dans le keyring
(un salt n'est pas un secret). Anti-bruteforce applicatif conservé (verrou progressif sur PIN faux).

## 4. Format du keyring (`keyring.json`)

Emplacement : `data dir` SituLab —
`%AppData%\SituLab\` (Windows) · `~/.config/SituLab/` (Linux) ·
`~/Library/Application Support/SituLab/` (macOS). Écriture **atomique** (fichier temp + rename),
comme la config v0.3.

```json
{
  "version": 1,
  "kdf": { "algo": "argon2id", "salt": "<b64>", "m": 65536, "t": 3, "p": 1 },
  "wrap": { "algo": "aes-256-gcm", "nonce": "<b64>", "ct": "<b64>" },
  "entries": [
    {
      "id": "apcom",
      "kind": "forgejo-token",
      "instance_url": "https://git.apcom.app",
      "scopes": ["read:repository", "write:repository"],
      "enc": { "algo": "aes-256-gcm", "nonce": "<b64>", "ct": "<b64>", "aad": "apcom" },
      "created_at": "2026-10-08T00:00:00Z"
    }
  ]
}
```

- `wrap` = DEK enveloppée par la KEK.
- `entries[].enc.ct` = valeur du token chiffrée sous la DEK.
- **Aucune valeur de token, aucun PIN, aucune KEK, aucune DEK en clair** dans ce fichier.
- `scopes` est indicatif (affichage), jamais une garantie de sécurité.

Le `config.json` v0.3 (instances, dépôts locaux suivis) reste **sans secret** ; il peut référencer
une entrée du keyring par `id`, mais ne contient aucun matériel cryptographique.

## 5. Cycle de vie en mémoire

- Au **déverrouillage** : KEK dérivée du PIN → DEK déballée → DEK gardée en mémoire protégée
  (buffer effaçable, non swappé si possible). La KEK et le PIN sont **effacés immédiatement** après
  avoir déballé la DEK.
- Pendant la **session déverrouillée** : les lectures déchiffrent le token nécessaire à la volée
  via la DEK en mémoire.
- Au **re-verrouillage** (timeout d'inactivité, fermeture, verrou manuel) : DEK et tout token
  déchiffré sont **zeroisés**.
- À la **fin d'une écriture** : le token déchiffré pour le push est zeroisé dès le retour de git.

## 6. Matrice des opérations × PIN

| Opération | Token requis | PIN requis |
|---|---|---|
| `version`, `doctor` (hors état coffre) | non | non |
| `instance list`, affichage config | non | non |
| **Lectures distantes** (`whoami`, `repo list/view/branches/tags/commits/tree/file`, `org list`, `token list`, `dashboard` partie distante) | oui | **session déverrouillée** |
| **Lectures/opérations locales** (`local status`, `dashboard` partie locale) | non (git seul) | non* |
| `repo clone`, « pull » d'un dépôt | token (selon droits) | **session déverrouillée** |
| **Écriture distante** (`repo push`) | oui | **confirmation PIN à chaque fois** |
| `instance add` (range un token) | — | **PIN** (chiffrer le token entrant) |
| **Création d'un token via SituLab** (API Forgejo) | token existant | **PIN** (déchiffrer l'appelant + chiffrer le token créé) |
| `pin set` / changement de PIN | — | PIN actuel (ré-enveloppe la DEK) |

\* Une opération purement locale (git sur un clone) n'a pas besoin du keyring. Mais une opération
locale qui **contacte le distant** (clone, pull, fetch authentifié) a besoin d'un token → session
déverrouillée.

### Règle des deux niveaux
- **Déverrouiller (lecture)** : un PIN par session ouvre l'accès aux lectures ; re-verrouillage sur
  timeout.
- **Confirmer (écriture)** : `repo push` redemande le PIN **à chaque appel**, même déverrouillé.
  C'est le geste humain explicite « j'autorise cette écriture ». En mode agent non-interactif,
  l'absence de PIN fait échouer le push avec le **code 3** (inchangé depuis v0.3).

## 7. Création de token depuis SituLab

Deux secrets à protéger dans la même opération :
1. **Appeler** l'API Forgejo « créer un token » exige un credential déjà présent → déchiffrement
   via la DEK (donc session déverrouillée / PIN).
2. Le **token neuf** renvoyé par Forgejo (visible une seule fois) doit être chiffré sous la DEK et
   écrit dans `entries[]` **avant** tout affichage ou log. Il n'apparaît jamais en clair sur disque,
   et `token list` n'en montrera que le nom et les portées.

## 8. Mode agent (non-interactif)

- Les **lectures** nécessitent une session déverrouillée. Un agent ne peut donc pas lire sans qu'un
  humain ait déverrouillé au préalable (choix assumé : l'humain reste dans la boucle même en
  lecture). Option de confort possible en v1.x : un **jeton de session** à durée de vie courte,
  passé explicitement par l'humain à l'agent, jamais persisté — à spécifier séparément.
- Les **écritures** échouent en **code 3** sans PIN. Pas de contournement par SituLab.
- Rappel honnête : SituLab ne peut pas empêcher un agent d'appeler `git` directement. La garantie
  v1 est que **le keyring ne livre aucun token sans le PIN** ; elle ne tient que si aucun token
  n'est par ailleurs rangé dans le coffre OS ou en clair ailleurs sur le poste. **Invariant :
  SituLab n'écrit jamais de token hors du keyring chiffré.**

## 9. Migration depuis v0.3

1. Au premier lancement v1 : détecter les tokens présents dans le coffre OS.
2. Demander à l'utilisateur de **définir son PIN** (crée salt + DEK + `wrap`).
3. Pour chaque token : le lire une dernière fois depuis le coffre OS, le chiffrer sous la DEK, le
   ranger dans `keyring.json`, puis le **supprimer du coffre OS**.
4. Vérifier qu'aucun token ne subsiste côté OS ; `doctor` le contrôle désormais.

## 10. État d'implémentation (livré depuis v1.1.0)

- [x] Module `internal/keyring` : format, chargement, écriture atomique, zeroisation.
- [x] Dérivation Argon2id → KEK, wrap/unwrap DEK, changement de PIN (dans `internal/keyring`).
      L'ancien paquet `internal/pin` a été supprimé : le consentement passe par le déballage du trousseau.
- [x] Session en mémoire : déverrouillage, timeout d'inactivité, verrou manuel, zeroisation.
- [x] `internal/vault` (coffre OS) : conservé **uniquement** pour la migration.
- [x] CLI/GUI : états *verrouillé / déverrouillé*, invite PIN à l'écriture, code 3 inchangé.
- [x] `doctor` : signale tout token résiduel dans le coffre OS (doit être vide).
- [x] Tests : wrap/unwrap, mauvais PIN → anti-bruteforce, atomicité, zeroisation, migration.

---

© 2026 APCOM Solutions SA.
