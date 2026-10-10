// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package cli

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"situlab/internal/attest"

	"github.com/spf13/cobra"
)

// Outillage d'ÉMISSION de la signature de garantie « SituLab approved » — geste de GOUVERNANCE, HORS
// LIGNE. Ces commandes manipulent la clé privée racine (et/ou intermédiaire) dans des FICHIERS que
// l'humain contrôle ; elles n'ouvrent jamais le démon, ne touchent jamais le trousseau ni le socket.
// À exécuter idéalement sur un poste isolé. Le privé racine se garde dans Vaultwarden (jamais sur le VPS).
//
// Chaîne (v1.9.2) : racine (hors ligne) → CA intermédiaire (émission) → certificat d'appareil (90 j).
//   1. gen-root  → paire racine ; la clé PUBLIQUE remplace internal/attest/apcom_root.pub (rebuild).
//   2. gen-ca    → autorité intermédiaire signée par la racine (la racine peut alors rester au coffre).
//   3. issue     → certificat d'appareil, signé par la racine (1 niveau) ou la CA (2 niveaux).
// Installation du cert d'appareil sur le poste cible : `situlab attest install-cert <cert.json>`.

func cmdAttestEmit() []*cobra.Command {
	// Une variable de sortie PAR commande : partager un seul `outPrefix` entre les trois sous-commandes
	// faisait écraser le défaut de chacune par le dernier StringVar enregistré (bug 1.9.2).
	var (
		outRoot   string
		outCA     string
		outCert   string
		outSW     string
		rootKey   string
		signerKey string
		issuerF   string
		devicePub string
		subject   string
		days      int
	)

	genRoot := &cobra.Command{
		Use:   "gen-root",
		Short: "GOUVERNANCE (hors ligne) : génère la paire racine APCOM (clé publique à embarquer, privé à garder au coffre)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			pub, priv, err := ed25519.GenerateKey(rand.Reader)
			if err != nil {
				return err
			}
			if outRoot == "" {
				outRoot = "apcom_root"
			}
			if err := writeSecret(outRoot+".key", b64(priv)); err != nil {
				return err
			}
			if err := os.WriteFile(outRoot+".pub", []byte(b64(pub)+"\n"), 0o644); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(),
				"Racine générée.\n  privé  : %s.key  🔴 à déposer dans Vaultwarden (safe.apcom.app), JAMAIS sur un serveur\n  public : %s.pub   → remplace internal/attest/apcom_root.pub puis rebuild\n\nClé publique racine (base64) :\n%s\n",
				outRoot, outRoot, b64(pub))
			return nil
		},
	}
	genRoot.Flags().StringVar(&outRoot, "out", "apcom_root", "préfixe des fichiers de sortie (.key privé, .pub public)")

	genCA := &cobra.Command{
		Use:   "gen-ca",
		Short: "GOUVERNANCE (hors ligne) : génère une autorité intermédiaire, signée par la racine",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			rootPriv, err := readKey(rootKey)
			if err != nil {
				return fmt.Errorf("clé racine : %w", err)
			}
			caPub, caPriv, err := ed25519.GenerateKey(rand.Reader)
			if err != nil {
				return err
			}
			nb := time.Now()
			na := nb.AddDate(0, 0, days)
			subj := subject
			if subj == "" {
				subj = "SituLab · APCOM Solutions SA · CA d'émission"
			}
			cert := attest.IssueCA(rootPriv, caPub, subj, nb, na, newSerial())
			if outCA == "" {
				outCA = "apcom_ca"
			}
			if err := writeSecret(outCA+".key", b64(caPriv)); err != nil {
				return err
			}
			if err := writeCert(outCA+".cert.json", cert); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(),
				"Autorité intermédiaire générée (valide %d j).\n  privé : %s.key  🔴 à garder au coffre (sert à `issue`)\n  cert  : %s.cert.json  (à passer en --issuer lors de l'émission)\n",
				days, outCA, outCA)
			return nil
		},
	}
	genCA.Flags().StringVar(&rootKey, "root-key", "", "fichier de la clé privée racine (gen-root)")
	genCA.Flags().StringVar(&subject, "subject", "", "sujet de l'autorité")
	genCA.Flags().IntVar(&days, "days", 365, "durée de validité en jours")
	genCA.Flags().StringVar(&outCA, "out", "apcom_ca", "préfixe des fichiers de sortie")
	_ = genCA.MarkFlagRequired("root-key")

	issue := &cobra.Command{
		Use:   "issue",
		Short: "GOUVERNANCE (hors ligne) : émet un certificat d'appareil (90 j) pour une clé publique donnée",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			signerPriv, err := readKey(signerKey)
			if err != nil {
				return fmt.Errorf("clé de signature : %w", err)
			}
			pub, err := readPub(devicePub)
			if err != nil {
				return fmt.Errorf("clé d'appareil : %w", err)
			}
			if subject == "" {
				return fmt.Errorf("--subject requis (ex. « SituLab · APCOM Solutions SA · Grégory Liand »)")
			}
			nb := time.Now()
			na := nb.AddDate(0, 0, days)
			var cert attest.Cert
			if issuerF != "" {
				issuer, err := readCert(issuerF)
				if err != nil {
					return fmt.Errorf("certificat émetteur : %w", err)
				}
				// Sûreté : la clé de signature doit être CELLE de la CA émettrice.
				if !signerPriv.Public().(ed25519.PublicKey).Equal(issuer.DevicePub) {
					return fmt.Errorf("--signer-key ne correspond pas à la clé de l'émetteur (--issuer)")
				}
				cert = attest.IssueCertFrom(signerPriv, issuer, pub, subject, nb, na, newSerial())
			} else {
				cert = attest.IssueCert(signerPriv, pub, subject, nb, na, newSerial())
			}
			out := outCert
			if out == "" {
				out = "device.cert.json"
			}
			if err := writeCert(out, cert); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(),
				"Certificat d'appareil émis (valide %d j) → %s\nInstaller sur le poste cible :\n  situlab attest install-cert %s --instance <connexion>\n",
				days, out, out)
			return nil
		},
	}
	issue.Flags().StringVar(&signerKey, "signer-key", "", "fichier de la clé privée signataire (racine, ou CA intermédiaire)")
	issue.Flags().StringVar(&issuerF, "issuer", "", "certificat de la CA émettrice (chaîne 2 niveaux) ; omis = signé par la racine")
	issue.Flags().StringVar(&devicePub, "device-pub", "", "clé publique d'appareil en base64 (ou @fichier) — issue de `attest genkey`")
	issue.Flags().StringVar(&subject, "subject", "", "sujet nominatif du certificat")
	issue.Flags().IntVar(&days, "days", 90, "durée de validité en jours")
	issue.Flags().StringVar(&outCert, "out", "device.cert.json", "fichier de sortie du certificat")
	_ = issue.MarkFlagRequired("signer-key")
	_ = issue.MarkFlagRequired("device-pub")

	genSoftware := &cobra.Command{
		Use:   "gen-software",
		Short: "GOUVERNANCE (hors ligne) : génère l'identité de SIGNATURE LOGICIELLE « SituLab » (une pour tout le logiciel)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			signerPriv, err := readKey(signerKey)
			if err != nil {
				return fmt.Errorf("clé de signature (CA ou racine) : %w", err)
			}
			swPub, swPriv, err := ed25519.GenerateKey(rand.Reader)
			if err != nil {
				return err
			}
			if subject == "" {
				subject = "SituLab · APCOM Solutions SA"
			}
			nb := time.Now()
			na := nb.AddDate(0, 0, days)
			var cert attest.Cert
			if issuerF != "" {
				issuer, err := readCert(issuerF)
				if err != nil {
					return fmt.Errorf("certificat émetteur : %w", err)
				}
				if !signerPriv.Public().(ed25519.PublicKey).Equal(issuer.DevicePub) {
					return fmt.Errorf("--signer-key ne correspond pas à la clé de l'émetteur (--issuer)")
				}
				cert = attest.IssueCertFrom(signerPriv, issuer, swPub, subject, nb, na, newSerial())
			} else {
				cert = attest.IssueCert(signerPriv, swPub, subject, nb, na, newSerial())
			}
			if outSW == "" {
				outSW = "situlab_sign"
			}
			if err := writeSecret(outSW+".key", b64(swPriv)); err != nil {
				return err
			}
			if err := writeCert(outSW+".cert.json", cert); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(),
				"Identité de signature LOGICIELLE générée (valide %d j, sujet « %s »).\n  %s.key + %s.cert.json\n→ À EMBARQUER dans le binaire : remplacer internal/attest/situlab_sign.key et .cert.json, puis rebuild.\n  La clé privée logicielle est alors DANS le binaire (garantie logiciel locale — limite assumée).\n",
				days, subject, outSW, outSW)
			return nil
		},
	}
	genSoftware.Flags().StringVar(&signerKey, "signer-key", "", "fichier de la clé privée signataire (CA intermédiaire, ou racine)")
	genSoftware.Flags().StringVar(&issuerF, "issuer", "", "certificat de la CA émettrice (chaîne 2 niveaux) ; omis = signé par la racine")
	genSoftware.Flags().StringVar(&subject, "subject", "", "sujet (défaut : « SituLab · APCOM Solutions SA »)")
	genSoftware.Flags().IntVar(&days, "days", 3650, "durée de validité en jours (défaut 10 ans ; renouvelée aux versions majeures)")
	genSoftware.Flags().StringVar(&outSW, "out", "situlab_sign", "préfixe des fichiers de sortie (.key + .cert.json)")
	_ = genSoftware.MarkFlagRequired("signer-key")

	return []*cobra.Command{genRoot, genCA, issue, genSoftware}
}

func b64(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

func newSerial() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// writeSecret écrit une clé privée en 0600 (lecture propriétaire seule).
func writeSecret(path, content string) error {
	return os.WriteFile(path, []byte(content+"\n"), 0o600)
}

func readKey(path string) (ed25519.PrivateKey, error) {
	if path == "" {
		return nil, fmt.Errorf("fichier manquant")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil {
		return nil, fmt.Errorf("clé illisible (base64) : %w", err)
	}
	if len(b) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("taille de clé privée invalide")
	}
	return ed25519.PrivateKey(b), nil
}

func readPub(s string) (ed25519.PublicKey, error) {
	v := strings.TrimSpace(s)
	if strings.HasPrefix(v, "@") {
		raw, err := os.ReadFile(v[1:])
		if err != nil {
			return nil, err
		}
		v = strings.TrimSpace(string(raw))
	}
	b, err := base64.StdEncoding.DecodeString(v)
	if err != nil {
		return nil, fmt.Errorf("clé publique illisible (base64) : %w", err)
	}
	if len(b) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("taille de clé publique invalide")
	}
	return ed25519.PublicKey(b), nil
}

func writeCert(path string, c attest.Cert) error {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

func readCert(path string) (attest.Cert, error) {
	var c attest.Cert
	raw, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	err = json.Unmarshal(raw, &c)
	return c, err
}
