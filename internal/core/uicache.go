package core

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"situlab/internal/config"
)

// Cache d'INTERFACE persistant (1.18.3) — stale-while-revalidate inter-sessions.
//
// But : au lancement, un écran peint d'abord le dernier état CONNU (ici, relu sur disque) au lieu
// d'une boîte vide, puis rafraîchit en arrière-plan. Supprime le « chemin froid » qui faisait
// attendre ~1 s à chaque premier affichage.
//
// Contenu : UNIQUEMENT des métadonnées de dépôt (listes de fichiers, tags, commits, issues, boards).
// JAMAIS de contenu CORPUS (souveraineté — Option A validée par la gouvernance) ni aucun secret.
//
// Nature : écriture LOCALE, best-effort, hors réseau. Ce n'est pas une mutation distante → pas de PIN,
// pas d'entrée dans daemon.isWriteOp. Vit sous config.Dir()/cache, donc Purge() le retire déjà.

const (
	uiCacheDirName  = "cache"
	uiCacheMaxValue = 1 << 20 // 1 Mio par entrée (au-delà : on ne persiste pas)
	uiCacheMaxFiles = 400     // plafond d'entrées ; au-delà, purge des plus anciennes (LRU par mtime)
)

var uiCacheMu sync.Mutex

func uiCacheDir() string { return filepath.Join(config.Dir(), uiCacheDirName) }

func uiCacheFile(key string) string {
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(uiCacheDir(), hex.EncodeToString(sum[:])+".json")
}

type uiCacheEnvelope struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	TS    int64  `json:"ts"`
}

// uiCacheForbidden : garde-fou de souveraineté. Le CORPUS (clouds souverains, droits par chemin,
// fail-closed) n'est JAMAIS écrit sur disque. Son cache reste en mémoire côté interface (Option A).
func uiCacheForbidden(key string) bool {
	k := strings.ToLower(strings.TrimSpace(key))
	return strings.HasPrefix(k, "corpus:") || strings.HasPrefix(k, "cloud:")
}

// UICacheLoad renvoie la valeur mémorisée pour la clé, ou "" si absente ou illisible (jamais une
// erreur bloquante : l'absence de cache est le cas normal au premier lancement).
func (s *Service) UICacheLoad(key string) (string, error) {
	if key == "" || uiCacheForbidden(key) {
		return "", nil
	}
	uiCacheMu.Lock()
	defer uiCacheMu.Unlock()
	b, err := os.ReadFile(uiCacheFile(key))
	if err != nil {
		return "", nil
	}
	var env uiCacheEnvelope
	if json.Unmarshal(b, &env) != nil || env.Key != key {
		return "", nil
	}
	return env.Value, nil
}

// UICacheSave mémorise une valeur pour la clé (best-effort). Refuse le CORPUS et les valeurs > 1 Mio.
func (s *Service) UICacheSave(key, value string) error {
	if key == "" || uiCacheForbidden(key) || len(value) > uiCacheMaxValue {
		return nil
	}
	uiCacheMu.Lock()
	defer uiCacheMu.Unlock()
	dir := uiCacheDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil
	}
	env := uiCacheEnvelope{Key: key, Value: value, TS: time.Now().Unix()}
	b, err := json.Marshal(env)
	if err != nil {
		return nil
	}
	target := uiCacheFile(key)
	tmp := target + ".tmp"
	if os.WriteFile(tmp, b, 0o600) == nil {
		_ = os.Rename(tmp, target)
	}
	uiCachePrune(dir)
	return nil
}

// uiCachePrune garde le dossier borné : au-delà du plafond, retire les entrées les plus anciennes.
func uiCachePrune(dir string) {
	ents, err := os.ReadDir(dir)
	if err != nil || len(ents) <= uiCacheMaxFiles {
		return
	}
	type fileAge struct {
		path string
		mod  time.Time
	}
	var files []fileAge
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		info, ie := e.Info()
		if ie != nil {
			continue
		}
		files = append(files, fileAge{filepath.Join(dir, e.Name()), info.ModTime()})
	}
	if len(files) <= uiCacheMaxFiles {
		return
	}
	sort.Slice(files, func(i, j int) bool { return files[i].mod.Before(files[j].mod) })
	for _, f := range files[:len(files)-uiCacheMaxFiles] {
		_ = os.Remove(f.path)
	}
}

// UICacheClear vide entièrement le cache d'interface. Appelable au besoin (la purge globale passe
// déjà par RemoveAll(config.Dir()) qui l'emporte).
func (s *Service) UICacheClear() error {
	uiCacheMu.Lock()
	defer uiCacheMu.Unlock()
	return os.RemoveAll(uiCacheDir())
}
