// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package config

// BackupPlan : sauvegarde d'un dépôt git local vers un dossier d'un cloud connecté (v1.10). Le
// backup est un bundle git (historique complet, un fichier restaurable). Exécution manuelle pour
// l'instant (planification automatique = étape suivante).
type BackupPlan struct {
	Name     string `json:"name"`
	RepoPath string `json:"repo_path,omitempty"` // (legacy mono-dépôt v1.10 ; vide pour les plans v1.14)
	Cloud    string `json:"cloud"`               // nom de la connexion cloud de destination
	Dest     string `json:"dest"`                // dossier de destination dans le cloud
	LastRun  string `json:"last_run,omitempty"`
	// --- Tunnel clé-en-main (v1.14) ---
	Type    string   `json:"type,omitempty"`    // "generic" (portée dynamique) | "targeted" (liste figée) ; vide = legacy
	Scope   string   `json:"scope,omitempty"`   // generic : "kernels" | "spokes" | "repos" | "kernels_spokes" | "all"
	Layer   string   `json:"layer,omitempty"`   // "local" | "atelier" | "both" (défaut "local") — dossier de dest du miroir (legacy)
	Source  string   `json:"source,omitempty"`  // SOURCE sauvegardée (1.19) : "local" | "atelier" | "vitrine" (défaut "local")
	Form    string   `json:"form,omitempty"`    // "bundle" (léger, re-clonable) | "mirror" (fichiers lisibles) ; défaut "bundle"
	Routine string   `json:"routine,omitempty"` // "manual" | "on_open" | "on_close" (défaut "manual")
	Repos   []string `json:"repos,omitempty"`   // targeted : chemins locaux inscrits (absolus, nettoyés)
	// Rétention (v1.17). Overwrite=false (défaut, et plans existants) = CUMULATIF : chaque exécution
	// garde son bundle horodaté, rien n'est supprimé (le cloud grossit). Overwrite=true = ÉCRASE :
	// seul le dernier bundle reste à la racine du dépôt, les précédents sont déplacés dans un
	// sous-dossier « archive/ » dont on ne garde que les Archives plus récents (0 = aucune archive).
	Overwrite bool `json:"overwrite,omitempty"`
	Archives  int  `json:"archives"` // 0..3 : nombre d'archives conservées quand Overwrite (PAS omitempty : 0 est un choix explicite « aucune archive » qui doit survivre à l'édition)
	// Sauvegarde automatique en arrière-plan (v1.18.2). Auto=true : l'ordonnanceur du process fenêtre
	// relance ce plan toutes les EveryMin minutes tant que SituLab est ouvert et déverrouillé, en
	// incrémental (ne sauve que les dépôts changés). EveryMin<=0 avec Auto → défaut 30.
	Auto     bool `json:"auto,omitempty"`
	EveryMin int  `json:"every_min,omitempty"`
}

// BackupRun : une entrée du JOURNAL de sauvegarde (v1.14). Non sensible.
type BackupRun struct {
	Plan    string `json:"plan"`
	When    string `json:"when"`              // RFC3339
	OK      bool   `json:"ok"`                // succès global
	Repos   int    `json:"repos"`             // nombre de dépôts traités
	Failed  int    `json:"failed,omitempty"`  // nombre d'échecs
	Form    string `json:"form,omitempty"`    // bundle | mirror
	Trigger string `json:"trigger,omitempty"` // manual | on_open | on_close | auto
	Detail  string `json:"detail,omitempty"`  // résumé court / 1re erreur
	// Journal détaillé (v1.18.2).
	Saved      int `json:"saved,omitempty"`       // dépôts réellement sauvegardés (changés)
	Skipped    int `json:"skipped,omitempty"`     // dépôts ignorés (inchangés depuis le dernier backup)
	DurationMs int `json:"duration_ms,omitempty"` // durée totale de l'exécution
}

// FindBackup renvoie un plan de sauvegarde par nom (insensible à la casse), ou nil.
func (c *Config) FindBackup(name string) *BackupPlan {
	for i := range c.Backups {
		if eqName(c.Backups[i].Name, name) {
			return &c.Backups[i]
		}
	}
	return nil
}

// RemoveBackup supprime un plan de sauvegarde par nom. Renvoie vrai si trouvé.
func (c *Config) RemoveBackup(name string) bool {
	out := c.Backups[:0]
	found := false
	for _, b := range c.Backups {
		if eqName(b.Name, name) {
			found = true
			continue
		}
		out = append(out, b)
	}
	c.Backups = out
	return found
}

// AddBackupRun ajoute une entrée au journal (en tête), plafonné aux 50 dernières.
func (c *Config) AddBackupRun(r BackupRun) {
	c.BackupRuns = append([]BackupRun{r}, c.BackupRuns...)
	if len(c.BackupRuns) > 50 {
		c.BackupRuns = c.BackupRuns[:50]
	}
}

// --- KERNEL (v1.11) ---------------------------------------------------------------------------
