// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

// Package journal tient un JOURNAL LOCAL des opérations de SituLab : quoi, quand, par quelle surface
// (fenêtre / cli / mcp), succès ou échec, erreur épurée, durée. But : rendre visibles les gestes des
// agents et les échecs « best-effort » aujourd'hui silencieux (sauvegarde, attestation…), sans AUCUNE
// télémétrie — tout reste sur le poste, dans un fichier JSONL rotatif. Le PIN et les jetons n'y entrent
// jamais (ils n'atteignent pas cette couche ; un filet de sécurité masque malgré tout une URL à jeton).
package journal

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Level fixe ce qui est RETENU : rien, seulement les échecs, ou tout.
type Level string

const (
	LevelOff    Level = "off"
	LevelErrors Level = "errors" // défaut : seuls les échecs sont persistés (faible bruit)
	LevelAll    Level = "all"
)

// NormalizeLevel borne une valeur libre sur un niveau connu (défaut : errors).
func NormalizeLevel(s string) Level {
	switch Level(strings.ToLower(strings.TrimSpace(s))) {
	case LevelOff:
		return LevelOff
	case LevelAll:
		return LevelAll
	default:
		return LevelErrors
	}
}

// Entry : une ligne de journal. Sérialisée en JSON (une par ligne, format JSONL).
type Entry struct {
	Time   time.Time `json:"t"`
	Actor  string    `json:"actor"`            // fenetre | cli | mcp
	Op     string    `json:"op"`               // opération (ex. push, repo.create)
	Target string    `json:"target,omitempty"` // dépôt/chemin concerné
	OK     bool      `json:"ok"`
	Err    string    `json:"err,omitempty"` // message épuré (vide si succès)
	DurMs  int64     `json:"ms,omitempty"`
	Intent string    `json:"intent,omitempty"` // « pourquoi » libre d'un agent, si fourni
}

// Journal écrit et relit le fichier. Concurrent-safe (mutex), best-effort (une écriture qui échoue ne
// remonte jamais à l'appelant : le journal ne doit jamais casser une opération).
type Journal struct {
	mu       sync.Mutex
	dir      string
	level    Level
	maxBytes int64
	keep     int
}

const (
	defaultMaxBytes = 1 << 20 // 1 Mio par fichier avant rotation
	defaultKeep     = 3       // journal.log + 3 archives
	fileName        = "journal.log"
)

// New construit un journal écrivant sous dir (créé au besoin).
func New(dir string, level Level) *Journal {
	return &Journal{dir: dir, level: NormalizeLevel(string(level)), maxBytes: defaultMaxBytes, keep: defaultKeep}
}

func (j *Journal) Level() Level {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.level
}

func (j *Journal) SetLevel(l Level) {
	j.mu.Lock()
	j.level = NormalizeLevel(string(l))
	j.mu.Unlock()
}

var redactURLCreds = regexp.MustCompile(`(://)[^@/\s]+@`)

func scrub(s string) string { return redactURLCreds.ReplaceAllString(s, "$1***@") }

// retain décide si une entrée est conservée au niveau courant.
func retain(level Level, ok bool) bool {
	switch level {
	case LevelOff:
		return false
	case LevelAll:
		return true
	default: // errors
		return !ok
	}
}

// Log enregistre une entrée si le niveau le demande. Jamais bloquant, jamais fatal.
func (j *Journal) Log(e Entry) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if !retain(j.level, e.OK) {
		return
	}
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	e.Err = scrub(e.Err)
	e.Intent = scrub(e.Intent)
	line, err := json.Marshal(e)
	if err != nil {
		return
	}
	if err := os.MkdirAll(j.dir, 0o700); err != nil {
		return
	}
	j.rotateIfNeeded(int64(len(line) + 1))
	f, err := os.OpenFile(j.path(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(line, '\n'))
}

func (j *Journal) path() string { return filepath.Join(j.dir, fileName) }

// rotateIfNeeded fait tourner le fichier courant quand il dépasserait le plafond : journal.log ->
// journal.1.log -> … -> journal.<keep>.log (le plus ancien est écrasé). Appelé sous mutex.
func (j *Journal) rotateIfNeeded(incoming int64) {
	fi, err := os.Stat(j.path())
	if err != nil || fi.Size()+incoming <= j.maxBytes {
		return
	}
	// décalage des archives, du plus ancien au plus récent
	for i := j.keep; i >= 1; i-- {
		older := filepath.Join(j.dir, archiveName(i))
		if i == j.keep {
			os.Remove(older)
			continue
		}
		_ = os.Rename(filepath.Join(j.dir, archiveName(i)), filepath.Join(j.dir, archiveName(i+1)))
	}
	_ = os.Rename(j.path(), filepath.Join(j.dir, archiveName(1)))
}

func archiveName(i int) string { return "journal." + itoa(i) + ".log" }

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [12]byte
	p := len(b)
	for i > 0 {
		p--
		b[p] = byte('0' + i%10)
		i /= 10
	}
	return string(b[p:])
}

// Read renvoie les DERNIÈRES entrées (les plus récentes d'abord), au plus limit. onlyErr ne garde que
// les échecs. Lit le fichier courant puis remonte dans les archives tant que nécessaire.
func (j *Journal) Read(limit int, onlyErr bool) ([]Entry, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if limit <= 0 {
		limit = 200
	}
	files := []string{j.path()}
	for i := 1; i <= j.keep; i++ {
		files = append(files, filepath.Join(j.dir, archiveName(i)))
	}
	// Ordre par ÉCRITURE (pas par horloge : sa résolution grossière collerait des entrées rapprochées).
	// Chaque fichier est chronologique (ancien → récent) ; on l'inverse (récent d'abord) et on enchaîne
	// du fichier courant vers les archives (plus anciennes). On s'arrête dès qu'on a de quoi servir limit.
	var out []Entry
	for _, fp := range files {
		ents, err := readFile(fp, onlyErr)
		if err != nil {
			continue // fichier absent : on ignore
		}
		for i := len(ents) - 1; i >= 0; i-- {
			out = append(out, ents[i])
		}
		if len(out) >= limit {
			break
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func readFile(fp string, onlyErr bool) ([]Entry, error) {
	f, err := os.Open(fp)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var ents []Entry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		var e Entry
		if json.Unmarshal(sc.Bytes(), &e) != nil {
			continue
		}
		if onlyErr && e.OK {
			continue
		}
		ents = append(ents, e)
	}
	return ents, nil
}

// Clear efface le journal courant et ses archives.
func (j *Journal) Clear() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	os.Remove(j.path())
	for i := 1; i <= j.keep; i++ {
		os.Remove(filepath.Join(j.dir, archiveName(i)))
	}
	return nil
}
