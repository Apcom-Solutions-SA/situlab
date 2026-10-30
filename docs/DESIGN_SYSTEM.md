# SituLab — Design-system & hiérarchie d'actions

> Doctrine unique de l'interface (1.18.3). Rien n'est inventé : on applique les normes établies
> (hiérarchie d'emphase Material / Apple HIG / Carbon ; icônes monochromes `currentColor` type
> Lucide/Feather). Toute surface (module, page, modale) s'y conforme, sans style « maison ».

## 1. Hiérarchie d'emphase des actions

| Forme | Classe(s) | Quand | Exemples |
|---|---|---|---|
| **Bouton primaire** | `.btn.btn-primary` | L'action principale **unique** d'une page ou d'une modale | « + Nouveau plan », « Relier un cloud », « Connecter une vitrine », bouton de confirmation d'une modale |
| **Bouton secondaire** | `.btn.btn-ghost` | Actions importantes mais non principales | « Modifier », « Ouvrir en ligne », « Réglages », « Journal » |
| **Lien texte** | `.lnk` | Navigation / action de faible poids, inline | « voir dans l'onglet… », « PIN oublié ? », « Accès IA », « Droits » |
| **Bouton-icône** | `.btn-icon` | Action **générique, répétée, compacte** en ligne/carte | supprimer, enregistrer, auto |

Règle : **une seule** action primaire par contexte. Le reste descend d'un cran.

## 2. Actions destructives

- Traitement **identique partout** : **icône poubelle monochrome** (`.btn-icon.danger`, `ico("trash")`)
  + **confirmation** (modale).
- Le **rouge** (`.btn-danger` / `.btn-primary.danger-btn`) est réservé au **bouton de confirmation**
  dans la modale de suppression. Jamais ailleurs, jamais le mot « Supprimer » en rouge dans une liste.

## 3. Icônes

- **Une seule bibliothèque** : le registre `ICON_PATHS` + `ico(nom[, taille])` (`00-core.js`).
  Trait 1.7, `viewBox 24`, `currentColor`, **monochrome**.
- **La couleur est réservée au SENS** (état de synchro, santé Git/Cloud, « en cours »), **jamais** à
  une icône d'action.
- **Aucun emoji** dans l'interface (rendu et coloré par l'OS = incohérence). Les emoji d'action sont
  remplacés par `ico(...)`.

## 4. Typographie des libellés

- **Casse unique** : capitale initiale (sentence case) pour tout libellé d'action/bouton/lien
  (« Repromouvoir », « Consulter », « Git »). Fin du tout-minuscule.
- Français partout (identifiants techniques exceptés).

## 5. Explorateur de fichiers (composant unique)

Toute navigation d'arbre (fichiers git, cloud CORPUS, sélection de fichier run, réglage de droits IA)
passe par **un seul composant** (`explorer.js`). Anatomie de rangée invariante :

`[ sélecteur? ] · [ icône de type ] · [ nom ] · [ méta : taille / date ] · [ actions ]`

- Fil d'Ariane unique, champ de filtre unique, set d'icônes unique, états vide/chargement unifiés.
- **Réactivité** : l'arbre d'un dépôt avec clone local se peint **depuis le disque, instantanément** ;
  la date de dernière publication et le marqueur de synchro affichent un **shimmer discret** jusqu'à
  confirmation serveur, puis se figent en douceur.
- **Variantes branchées** selon le contexte : lecture seule · multi-sélection (cases) · sélection
  unique · droits (œil visible/masqué, arbre dépliable) · slot d'actions par rangée.

## 6. Animation & finition

- Transitions discrètes mais abouties (`--t-fast`, `--t-mid`), apparition en fondu (`.sl-settle`),
  shimmer (`.sl-cell-load`, `.sl-refreshing`). **Aucun saut de layout** : le contenu instantané et le
  contenu rafraîchi occupent la même place. Respect de `prefers-reduced-motion`.

---

*Référence appliquée à 100 % de l'interface en 1.18.3. Toute nouvelle surface s'y conforme.*
