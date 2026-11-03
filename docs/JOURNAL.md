# Journal local des opérations

SituLab tient un **journal local** des opérations : quoi, quand, par quelle surface,
succès ou échec, erreur épurée et durée. Il répond à deux besoins :

- **Comprendre ce qu'un agent a fait** (gestes CLI/MCP), dans l'esprit « l'humain au-dessus de ses IA ».
- **Voir les échecs silencieux** des gestes faits au mieux (sauvegarde automatique, attestation…) qui,
  sinon, ne disent rien.

## Principes

- **Local, jamais envoyé.** Le journal vit dans un fichier sur le poste. Ce n'est **pas** de la
  télémétrie : rien ne quitte la machine.
- **Sans secret.** Le PIN et les jetons n'atteignent pas cette couche ; un filet de sécurité masque en
  plus toute URL contenant un identifiant (`https://***@…`).
- **Au mieux.** Écrire au journal n'échoue jamais une opération et ne bloque jamais.

## Emplacement et rotation

- Dossier : `%APPDATA%\SituLab\journal\` (Windows) — `journal.log` + archives `journal.1.log`…
- Format : **JSONL** (une entrée JSON par ligne).
- Rotation automatique (~1 Mio par fichier, 3 archives conservées) : le journal ne grossit pas sans fin.

Chaque entrée : `t` (horodatage), `actor` (`fenetre` | `cli` | `mcp`), `op`, `target` (dépôt/chemin),
`ok`, `err` (épurée), `ms` (durée), `intent` (le « pourquoi » libre d'un agent, s'il l'a fourni).

## Niveaux

| Niveau | Ce qui est retenu |
|---|---|
| `off` | rien |
| `errors` | **défaut** — seulement les échecs (faible bruit) |
| `all` | toutes les opérations (pour diagnostiquer) |

## Consulter / régler

Trois surfaces, même journal :

- **Fenêtre** : Réglages → **Journaux** (niveau, liste des dernières entrées, effacement).
- **CLI** :
  - `situlab journal tail [--limit N] [--errors]` — dernières entrées (récentes d'abord) ;
  - `situlab journal level [off|errors|all]` — afficher ou changer le niveau ;
  - `situlab journal clear` — effacer.
- **MCP** : outil `journal_read` (lecture seule ; `limit`, `errors_only`). Un agent **voit** le journal,
  il ne le purge pas.

Le changement de niveau et l'effacement passent par le démon résident, pour que l'enregistrement en
cours reflète aussitôt le réglage.
