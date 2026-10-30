package core

import "situlab/internal/gitx"

// Arbre du clone LOCAL pour l'explorateur (1.18.3) — lecture disque, hors-ligne. Permet de peindre
// l'onglet Fichiers instantanément avant la réconciliation avec le dépôt en ligne. Non muté : pas de
// PIN, pas de portillon (même nature que LocalDetail).

// LocalTreeAt liste le contenu direct d'un sous-dossier du clone (depuis HEAD).
func (s *Service) LocalTreeAt(dir, subpath string) ([]gitx.TreeEntry, error) {
	return gitx.LocalTreeAt(dir, subpath)
}

// LocalTreeDates renvoie la date du dernier commit par chemin (best-effort, en arrière-plan).
func (s *Service) LocalTreeDates(dir string, paths []string) map[string]string {
	return gitx.LocalTreeDates(dir, paths)
}

// LocalTreeTotals : tailles agrégées des dossiers enfants + total du sous-arbre (poids total du dépôt
// à la racine). Hors-ligne, un seul appel git.
func (s *Service) LocalTreeTotals(dir, subpath string) (gitx.TreeTotals, error) {
	return gitx.LocalTreeTotals(dir, subpath)
}
