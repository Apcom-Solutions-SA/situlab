// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package config

// Kernel désigne un dépôt git local comme « mémoire » (KERNEL) de l'utilisateur (v1.11) : le dépôt
// qui EST la mémoire, distingué des dépôts logiciels ordinaires. Plusieurs KERNEL peuvent coexister
// (plusieurs sphères) ; un seul est « actif » à la fois (Config.ActiveKernel). Aucun secret : le
// jeton d'accès reste celui de l'instance git (trousseau), comme pour tout dépôt. La DÉSIGNATION
// (ajout/retrait/actif) est un geste de gouvernance réservé à la FENÊTRE (jamais socket/CLI/MCP).
type Kernel struct {
	Name       string `json:"name"`                  // libellé humain (« Ma mémoire APCOM »)
	LocalPath  string `json:"local_path"`            // clone local qui porte la mémoire
	Instance   string `json:"instance,omitempty"`    // instance git pour récupérer/enregistrer (vide = défaut)
	Backup     string `json:"backup,omitempty"`      // nom d'un BackupPlan rattaché (sauvegarde vers un cloud CORPUS)
	RunFile    string `json:"run_file,omitempty"`    // fichier d'ouverture pour l'IA locale (chemin relatif au dépôt)
	SpokesFile string `json:"spokes_file,omitempty"` // fichier de GESTION des spokes (procédure de consolidation), chemin relatif
}

// Spoke désigne un dépôt local comme PROJECTION d'une mission (SPOKE, v1.11) dérivée d'un KERNEL.
// Ce qui distingue un spoke d'un dépôt ordinaire : il référence le KERNEL dont il dérive et porte une
// mission (intention). La DÉSIGNATION est, comme pour le KERNEL, réservée à la FENÊTRE.
type Spoke struct {
	Name      string `json:"name"`
	LocalPath string `json:"local_path"`
	Kernel    string `json:"kernel,omitempty"`   // nom du KERNEL parent dont ce spoke dérive
	Mission   string `json:"mission,omitempty"`  // à quoi sert ce spoke (intention)
	Instance  string `json:"instance,omitempty"` // instance git pour récupérer/enregistrer (vide = défaut)
	RunFile   string `json:"run_file,omitempty"` // fichier d'ouverture (run) pour l'IA locale (chemin relatif)
}

// FindKernel renvoie le KERNEL par nom (insensible à la casse), ou nil.
func (c *Config) FindKernel(name string) *Kernel {
	for i := range c.Kernels {
		if eqName(c.Kernels[i].Name, name) {
			return &c.Kernels[i]
		}
	}
	return nil
}

// ActiveKernelRef renvoie le KERNEL actif (ActiveKernel), à défaut le premier, ou nil.
func (c *Config) ActiveKernelRef() *Kernel {
	if c.ActiveKernel != "" {
		if k := c.FindKernel(c.ActiveKernel); k != nil {
			return k
		}
	}
	if len(c.Kernels) > 0 {
		return &c.Kernels[0]
	}
	return nil
}

// ResolveKernel sélectionne un KERNEL par nom ; nom vide => KERNEL actif.
func (c *Config) ResolveKernel(name string) *Kernel {
	if name == "" {
		return c.ActiveKernelRef()
	}
	return c.FindKernel(name)
}

// SetActiveKernel marque un KERNEL comme actif (nom canonique). Renvoie vrai si trouvé.
func (c *Config) SetActiveKernel(name string) bool {
	k := c.FindKernel(name)
	if k == nil {
		return false
	}
	c.ActiveKernel = k.Name
	return true
}

// RemoveKernel supprime un KERNEL par nom. Si c'était l'actif, promeut le premier restant.
func (c *Config) RemoveKernel(name string) bool {
	out := c.Kernels[:0]
	removed, wasActive := "", false
	for _, k := range c.Kernels {
		if eqName(k.Name, name) {
			removed, wasActive = k.Name, eqName(c.ActiveKernel, k.Name)
			continue
		}
		out = append(out, k)
	}
	c.Kernels = out
	if removed == "" {
		return false
	}
	if wasActive {
		c.ActiveKernel = ""
		if len(c.Kernels) > 0 {
			c.ActiveKernel = c.Kernels[0].Name
		}
	}
	return true
}

// --- SPOKES (v1.11) ---------------------------------------------------------------------------

// FindSpoke renvoie le spoke par nom (insensible à la casse), ou nil.
func (c *Config) FindSpoke(name string) *Spoke {
	for i := range c.Spokes {
		if eqName(c.Spokes[i].Name, name) {
			return &c.Spokes[i]
		}
	}
	return nil
}

// RemoveSpoke supprime un spoke par nom. Renvoie vrai si trouvé.
func (c *Config) RemoveSpoke(name string) bool {
	out := c.Spokes[:0]
	found := false
	for _, sp := range c.Spokes {
		if eqName(sp.Name, name) {
			found = true
			continue
		}
		out = append(out, sp)
	}
	c.Spokes = out
	return found
}
