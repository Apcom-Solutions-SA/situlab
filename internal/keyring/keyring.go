// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

// Package keyring range les tokens d'instance dans un trousseau LOCAL CHIFFRÉ dont la
// seule clé est le PIN de l'utilisateur. Il remplace l'ancien coffre natif de l'OS
// (internal/vault) à partir de SituLab v1.1.0.
//
// Modèle (cf. docs/ARCHITECTURE-SECURITE-v1.md) :
//
//	PIN ──Argon2id(salt, params)──▶ KEK (clé de chiffrement de clés, jamais persistée)
//	DEK = 32 octets aléatoires (CSPRNG)        (clé de données, chiffre les tokens)
//	wrap = AES-256-GCM(KEK, DEK)               (seule forme persistée de la DEK)
//	token_i chiffré = AES-256-GCM(DEK, token_i, aad=id)
//
// Conséquences :
//   - sans le PIN, le trousseau est illisible — pour SituLab comme pour tout agent
//     tournant sous la session de l'utilisateur ;
//   - le PIN n'est stocké nulle part : sa validité se prouve en déballant la DEK (le
//     tag d'authentification GCM EST le vérificateur) — plus besoin d'un condensé ;
//   - changer de PIN ré-enveloppe la DEK seulement, jamais les tokens.
package keyring

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/crypto/argon2"

	"situlab/internal/config"
)

// Version du format de fichier (le bloc kdf reste auto-descriptif pour migrer sans casse).
const Version = 1

// Paramètres Argon2id actés le 2026-10-08 : cette clé (KEK) protège les tokens AU REPOS. La résistance
// hors-ligne d'un fichier VOLÉ dépend du COÛT Argon2id ET de l'entropie du PIN : un PIN à 6 chiffres
// (~10^6 candidats) reste cassable en minutes-heures malgré Argon2id. Pour un secret sensible, préférer
// un PIN plus long ; et chiffrer le disque (BitLocker/FileVault). Paramètres dans le fichier (migrables).
const (
	defMemKiB = 64 * 1024 // 64 MiB
	defTime   = 3
	defPar    = 4
	keyLen    = 32 // AES-256
	saltLen   = 16
	algoKDF   = "argon2id"
	algoAEAD  = "aes-256-gcm"
)

// Étiquettes AAD : lient un chiffré à son rôle/emplacement (anti-rejeu, anti-échange).
const aadWrap = "situlab-keyring-wrap-v1"

var (
	// ErrNotInitialized : aucun trousseau sur ce poste (pas encore de PIN défini).
	ErrNotInitialized = errors.New("trousseau non initialisé : définissez d'abord un PIN")
	// ErrAlreadyInit : tentative d'initialiser un trousseau déjà présent.
	ErrAlreadyInit = errors.New("un trousseau existe déjà")
	// ErrBadPIN : le PIN ne déballe pas la DEK (PIN faux, ou wrap altéré).
	ErrBadPIN = errors.New("PIN incorrect")
	// ErrNoEntry : aucun token pour cet identifiant d'instance.
	ErrNoEntry = errors.New("aucun token pour cette instance")
	// ErrFormat : fichier de trousseau illisible ou corrompu.
	ErrFormat = errors.New("trousseau illisible ou corrompu")
)

// blob est un chiffré AES-256-GCM auto-descriptif (les []byte sont encodés en base64
// par encoding/json).
type blob struct {
	Algo  string `json:"algo"`
	Nonce []byte `json:"nonce"`
	CT    []byte `json:"ct"`
}

// kdfParams décrit la dérivation du PIN vers la KEK (le sel n'est pas un secret).
type kdfParams struct {
	Algo string `json:"algo"`
	Salt []byte `json:"salt"`
	Mem  uint32 `json:"m"`
	Time uint32 `json:"t"`
	Par  uint8  `json:"p"`
}

// Entry est un token d'instance chiffré. Scopes/InstanceURL sont indicatifs (affichage),
// jamais une garantie de sécurité.
type Entry struct {
	ID          string    `json:"id"`
	InstanceURL string    `json:"instance_url"`
	Scopes      []string  `json:"scopes,omitempty"`
	Enc         blob      `json:"enc"`
	Pwd         *blob     `json:"pwd,omitempty"`  // mot de passe mémorisé (comptes SANS 2FA), chiffré sous la DEK
	Sign        *blob     `json:"sign,omitempty"` // clé privée d'appareil (signature de garantie), chiffrée sous la DEK
	CreatedAt   time.Time `json:"created_at"`
}

// File est le trousseau persistant. Il ne contient AUCUN secret en clair : ni PIN, ni
// KEK, ni DEK, ni valeur de token.
type File struct {
	Version int       `json:"version"`
	KDF     kdfParams `json:"kdf"`
	Wrap    blob      `json:"wrap"` // DEK enveloppée par la KEK
	Entries []Entry   `json:"entries"`
}

// Path renvoie le chemin du trousseau (même dossier que config.json).
func Path() string { return filepath.Join(config.Dir(), "keyring.json") }

// Scopes renvoie les portées enregistrées pour une instance (indicatif, non chiffré — pas besoin
// de la DEK). Sert à savoir si le jeton porte déjà le droit d'administration.
func (f *File) Scopes(id string) []string {
	for i := range f.Entries {
		if f.Entries[i].ID == id {
			return f.Entries[i].Scopes
		}
	}
	return nil
}

// Initialized indique si un trousseau existe déjà sur ce poste.
func Initialized() bool {
	_, err := os.Stat(Path())
	return err == nil
}

// Load lit le trousseau. Absence => ErrNotInitialized ; fichier illisible => ErrFormat.
func Load() (*File, error) {
	data, err := os.ReadFile(Path())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotInitialized
		}
		return nil, err
	}
	f := &File{}
	if err := json.Unmarshal(data, f); err != nil {
		return nil, ErrFormat
	}
	// Version EXACTE (un fichier d'une version future ne doit pas être réécrit en v1, ce qui
	// perdrait silencieusement ses champs inconnus) et algo attendu.
	if f.Version != Version || f.KDF.Algo != algoKDF {
		return nil, ErrFormat
	}
	// Bornes des paramètres Argon2id : des valeurs hors plage (corruption disque, altération)
	// feraient PANIQUER argon2.IDKey au déverrouillage et tomber tout le démon. On refuse
	// proprement en ErrFormat plutôt que de paniquer (contrat de Load).
	if !validKDF(f.KDF) {
		return nil, ErrFormat
	}
	return f, nil
}

// validKDF vérifie que les paramètres tiennent dans des plages sûres pour argon2.IDKey
// (parallelism ≥ 1, iterations ≥ 1, mémoire ≥ 8*p KiB, et plafonds anti-OOM/anti-gel).
func validKDF(k kdfParams) bool {
	if len(k.Salt) < 8 || len(k.Salt) > 64 {
		return false
	}
	if k.Par < 1 || k.Par > 64 {
		return false
	}
	if k.Time < 1 || k.Time > 64 {
		return false
	}
	// Mémoire en KiB : au moins 8*p (exigence argon2) et 8 MiB, au plus 2 GiB (anti-OOM).
	if k.Mem < 8*1024 || k.Mem < 8*uint32(k.Par) || k.Mem > 2*1024*1024 {
		return false
	}
	return true
}

// Init crée un trousseau neuf verrouillé par pin et renvoie le fichier ainsi que la DEK
// en clair (à garder en mémoire pour la session en cours). Échoue si un trousseau existe.
func Init(pin string) (*File, []byte, error) {
	if Initialized() {
		return nil, nil, ErrAlreadyInit
	}
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return nil, nil, err
	}
	dek := make([]byte, keyLen)
	if _, err := rand.Read(dek); err != nil {
		return nil, nil, err
	}
	k := kdfParams{Algo: algoKDF, Salt: salt, Mem: defMemKiB, Time: defTime, Par: defPar}
	kek := deriveKEK(pin, k)
	defer zero(kek)
	wrap, err := seal(kek, dek, []byte(aadWrap))
	if err != nil {
		zero(dek)
		return nil, nil, err
	}
	f := &File{Version: Version, KDF: k, Wrap: wrap, Entries: nil}
	if err := f.save(); err != nil {
		zero(dek)
		return nil, nil, err
	}
	return f, dek, nil
}

// Unlock dérive la KEK depuis pin et déballe la DEK. Un PIN faux renvoie ErrBadPIN
// (le tag GCM échoue). La DEK renvoyée doit être effacée (zero) à la fin de la session.
func (f *File) Unlock(pin string) ([]byte, error) {
	kek := deriveKEK(pin, f.KDF)
	defer zero(kek)
	dek, err := open(kek, f.Wrap, []byte(aadWrap))
	if err != nil {
		return nil, ErrBadPIN
	}
	return dek, nil
}

// SetToken chiffre (ou remplace) le token d'une instance sous la DEK, puis persiste.
func (f *File) SetToken(dek []byte, id, instanceURL string, scopes []string, token string) error {
	enc, err := seal(dek, []byte(token), tokenAAD(id))
	if err != nil {
		return err
	}
	replaced := false
	for i := range f.Entries {
		if f.Entries[i].ID == id {
			// Remplacement : on ne touche QUE les champs liés au jeton. On PRÉSERVE les secrets frères de
			// l'entrée (Pwd = mot de passe mémorisé ; Sign = clé de signature d'appareil) et la date de
			// première création — sinon une simple reconnexion détruirait silencieusement la clé de
			// signature (sceau de garantie cassé) ou le « se souvenir du mot de passe ».
			f.Entries[i].InstanceURL = instanceURL
			f.Entries[i].Scopes = scopes
			f.Entries[i].Enc = enc
			replaced = true
			break
		}
	}
	if !replaced {
		f.Entries = append(f.Entries, Entry{ID: id, InstanceURL: instanceURL, Scopes: scopes, Enc: enc, CreatedAt: time.Now().UTC()})
	}
	return f.save()
}

// GetToken déchiffre le token d'une instance avec la DEK. L'appelant efface la valeur
// après usage.
func (f *File) GetToken(dek []byte, id string) (string, error) {
	for i := range f.Entries {
		if f.Entries[i].ID == id {
			pt, err := openDomain(dek, f.Entries[i].Enc, tokenAAD(id), tokenAADLegacy(id))
			if err != nil {
				return "", ErrFormat // DEK valide mais entrée altérée
			}
			return string(pt), nil
		}
	}
	return "", ErrNoEntry
}

// DeleteToken retire le token d'une instance (aucun secret requis). Idempotent.
func (f *File) DeleteToken(id string) error {
	out := f.Entries[:0]
	for _, e := range f.Entries {
		if e.ID != id {
			out = append(out, e)
		}
	}
	f.Entries = out
	return f.save()
}

// SetPassword mémorise (ou efface si pwd="") le mot de passe du compte pour une instance,
// chiffré sous la DEK (AAD distincte du token). Réservé aux comptes SANS 2FA (le PIN suffit
// alors pour gérer les jetons ; un code 2FA, lui, n'est pas mémorisable).
func (f *File) SetPassword(dek []byte, id, pwd string) error {
	for i := range f.Entries {
		if f.Entries[i].ID == id {
			if pwd == "" {
				f.Entries[i].Pwd = nil
			} else {
				b, err := seal(dek, []byte(pwd), pwdAAD(id))
				if err != nil {
					return err
				}
				f.Entries[i].Pwd = &b
			}
			return f.save()
		}
	}
	return ErrNoEntry
}

// GetPassword renvoie le mot de passe mémorisé d'une instance ("" si aucun).
func (f *File) GetPassword(dek []byte, id string) (string, error) {
	for i := range f.Entries {
		if f.Entries[i].ID == id {
			if f.Entries[i].Pwd == nil {
				return "", nil
			}
			pt, err := openDomain(dek, *f.Entries[i].Pwd, pwdAAD(id), pwdAADLegacy(id))
			if err != nil {
				return "", ErrFormat
			}
			return string(pt), nil
		}
	}
	return "", ErrNoEntry
}

// SetSignKey mémorise (ou efface si priv vide) la clé privée d'appareil d'une instance, chiffrée sous
// la DEK. Le privé ne doit jamais sortir du démon.
func (f *File) SetSignKey(dek []byte, id string, priv []byte) error {
	for i := range f.Entries {
		if f.Entries[i].ID == id {
			if len(priv) == 0 {
				f.Entries[i].Sign = nil
			} else {
				b, err := seal(dek, priv, signAAD(id))
				if err != nil {
					return err
				}
				f.Entries[i].Sign = &b
			}
			return f.save()
		}
	}
	return ErrNoEntry
}

// GetSignKey renvoie la clé privée d'appareil d'une instance ("" si aucune). L'appelant efface la
// valeur après usage.
func (f *File) GetSignKey(dek []byte, id string) ([]byte, error) {
	for i := range f.Entries {
		if f.Entries[i].ID == id {
			if f.Entries[i].Sign == nil {
				return nil, nil
			}
			pt, err := open(dek, *f.Entries[i].Sign, signAAD(id))
			if err != nil {
				return nil, ErrFormat
			}
			return pt, nil
		}
	}
	return nil, ErrNoEntry
}

// Has indique si une instance a un token rangé.
func (f *File) Has(id string) bool {
	for i := range f.Entries {
		if f.Entries[i].ID == id {
			return true
		}
	}
	return false
}

// Rewrap ré-enveloppe une DEK DÉJÀ déballée sous un nouveau PIN (nouveau sel + KEK), puis
// persiste. Évite de re-dériver l'ancien PIN quand l'appelant a déjà la DEK en session.
// Les tokens restent inchangés (la DEK ne change pas).
func (f *File) Rewrap(dek []byte, newPIN string) error {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return err
	}
	k := kdfParams{Algo: algoKDF, Salt: salt, Mem: defMemKiB, Time: defTime, Par: defPar}
	kek := deriveKEK(newPIN, k)
	defer zero(kek)
	wrap, err := seal(kek, dek, []byte(aadWrap))
	if err != nil {
		return err
	}
	f.KDF = k
	f.Wrap = wrap
	return f.save()
}

// --- primitives ---

func deriveKEK(pin string, k kdfParams) []byte {
	return argon2.IDKey([]byte(pin), k.Salt, k.Time, k.Mem, k.Par, keyLen)
}

func seal(key, plaintext, aad []byte) (blob, error) {
	g, err := gcm(key)
	if err != nil {
		return blob{}, err
	}
	nonce := make([]byte, g.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return blob{}, err
	}
	return blob{Algo: algoAEAD, Nonce: nonce, CT: g.Seal(nil, nonce, plaintext, aad)}, nil
}

func open(key []byte, b blob, aad []byte) ([]byte, error) {
	g, err := gcm(key)
	if err != nil {
		return nil, err
	}
	if len(b.Nonce) != g.NonceSize() {
		return nil, ErrFormat
	}
	return g.Open(nil, b.Nonce, b.CT, aad)
}

// Domaines AAD : lient chaque secret à son usage (token vs mot de passe) ET à son instance, pour
// qu'un blob ne puisse jamais être déplacé d'un champ ou d'une entrée à l'autre. Le séparateur NUL
// lève toute ambiguïté de concaténation (un id d'instance n'en contient pas).
func tokenAAD(id string) []byte { return []byte("situlab\x00token\x00" + id) }
func pwdAAD(id string) []byte   { return []byte("situlab\x00pwd\x00" + id) }

// AAD historiques (avant v1.8) — en LECTURE SEULE, pour ne pas invalider un trousseau existant.
func tokenAADLegacy(id string) []byte { return []byte(id) }
func pwdAADLegacy(id string) []byte   { return []byte(id + ":pwd") }

// signAAD : domaine de la clé privée d'appareil (signature de garantie). Nouveau depuis le départ,
// donc versionné et sans repli historique.
func signAAD(id string) []byte { return []byte("situlab\x00signkey\x00" + id) }

// openDomain déchiffre avec l'AAD courant, puis retombe sur l'AAD historique (blob écrit par une
// version antérieure). La prochaine écriture rescelle avec l'AAD courant.
func openDomain(key []byte, b blob, primary, legacy []byte) ([]byte, error) {
	pt, err := open(key, b, primary)
	if err == nil {
		return pt, nil
	}
	return open(key, b, legacy)
}

func gcm(key []byte) (cipher.AEAD, error) {
	c, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(c)
}

func zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

// --- écriture atomique ---

// save écrit le trousseau de façon atomique (temp + rename, chmod 0600), comme config.go.
func (f *File) save() error {
	dir := config.Dir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "keyring-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op si le rename a réussi
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, Path())
}
