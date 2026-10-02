// Package pairing garde les jetons des applications associees a l'agent.
//
// L'agent ecoute sur 127.0.0.1, et toute page ouverte sur le poste peut appeler 127.0.0.1. La
// liste d'origines arrete un site quelconque ; le jeton est la seconde serrure : seule une page
// qui s'est associee, depuis une origine autorisee, peut lister les imprimantes, imprimer ou
// ouvrir le tiroir.
//
// L'application genere elle-meme un jeton aleatoire et l'envoie a POST /pair. L'agent n'en garde
// que l'empreinte SHA-256 : le fichier des associations ne permet pas de se faire passer pour
// l'application. Plusieurs jetons coexistent, un par profil de navigateur ou par application.
//
// La barre de notification n'est pas une page web : elle passe par un jeton local, ecrit en
// clair dans le dossier de l'agent, qu'aucun site ne peut lire.
package pairing

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	// Header porte le jeton sur chaque appel.
	Header = "X-Print-Bridge-Token"

	pairingsFile   = "pairings.json"
	LocalTokenFile = "local-token"

	minTokenLen = 32
	maxTokenLen = 512
	// Au-dela, l'association la plus ancienne cede la place : un poste ne compte pas des
	// dizaines de profils, et la liste ne doit pas grossir sans fin.
	maxPairings = 32
)

// ErrInvalidToken signale un jeton trop court, trop long ou non imprimable.
var ErrInvalidToken = errors.New("jeton invalide : 32 a 512 caracteres imprimables")

type entry struct {
	Hash     string    `json:"sha256"`
	Origin   string    `json:"origin,omitempty"`
	PairedAt time.Time `json:"pairedAt"`
}

type fileFormat struct {
	Pairings []entry `json:"pairings"`
}

// Store garde les empreintes en memoire et dans pairings.json.
type Store struct {
	mu         sync.Mutex
	dir        string
	entries    []entry
	localHash  string
	localToken string
}

// Open lit les associations du dossier dir et prepare le jeton local, cree au premier lancement.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	s := &Store{dir: dir}
	raw, err := os.ReadFile(filepath.Join(dir, pairingsFile))
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return nil, err
	default:
		var f fileFormat
		if err := json.Unmarshal(raw, &f); err != nil {
			return nil, fmt.Errorf("%s illisible : %w", pairingsFile, err)
		}
		s.entries = f.Pairings
	}
	local, err := ensureLocalToken(filepath.Join(dir, LocalTokenFile))
	if err != nil {
		return nil, err
	}
	s.localToken = local
	s.localHash = hash(local)
	return s, nil
}

// ReadLocalToken lit le jeton local du dossier dir, pour la barre de notification.
func ReadLocalToken(dir string) string {
	raw, err := os.ReadFile(filepath.Join(dir, LocalTokenFile))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

// Validate dit si token a la forme d'un jeton acceptable.
func Validate(token string) error {
	if len(token) < minTokenLen || len(token) > maxTokenLen {
		return ErrInvalidToken
	}
	for _, r := range token {
		if r <= ' ' || r > '~' {
			return ErrInvalidToken
		}
	}
	return nil
}

// Pair enregistre token pour origin. Associer deux fois le meme jeton ne cree pas de doublon.
func (s *Store) Pair(token, origin string) error {
	if err := Validate(token); err != nil {
		return err
	}
	h := hash(token)
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := s.entries[:0:0]
	for _, e := range s.entries {
		if e.Hash != h {
			kept = append(kept, e)
		}
	}
	kept = append(kept, entry{Hash: h, Origin: origin, PairedAt: time.Now().UTC()})
	if len(kept) > maxPairings {
		kept = kept[len(kept)-maxPairings:]
	}
	if err := s.save(kept); err != nil {
		return err
	}
	s.entries = kept
	return nil
}

// Unpair retire token ; faux s'il n'etait pas associe. Le jeton local ne se retire pas.
func (s *Store) Unpair(token string) (bool, error) {
	h := hash(token)
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := s.entries[:0:0]
	found := false
	for _, e := range s.entries {
		if e.Hash == h {
			found = true
			continue
		}
		kept = append(kept, e)
	}
	if !found {
		return false, nil
	}
	if err := s.save(kept); err != nil {
		return false, err
	}
	s.entries = kept
	return true, nil
}

// Valid dit si token est associe, ou s'il est le jeton local.
func (s *Store) Valid(token string) bool {
	if token == "" {
		return false
	}
	h := []byte(hash(token))
	s.mu.Lock()
	defer s.mu.Unlock()
	ok := subtle.ConstantTimeCompare(h, []byte(s.localHash)) == 1
	for _, e := range s.entries {
		if subtle.ConstantTimeCompare(h, []byte(e.Hash)) == 1 {
			ok = true
		}
	}
	return ok
}

// IsLocal dit si token est le jeton local.
func (s *Store) IsLocal(token string) bool {
	return token != "" && subtle.ConstantTimeCompare([]byte(hash(token)), []byte(s.localHash)) == 1
}

// LocalToken rend le jeton local.
func (s *Store) LocalToken() string { return s.localToken }

// save ecrit d'abord un fichier temporaire puis le renomme : une coupure au mauvais moment ne
// laisse pas un fichier tronque qui ferait perdre toutes les associations.
func (s *Store) save(entries []entry) error {
	raw, err := json.MarshalIndent(fileFormat{Pairings: entries}, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(s.dir, pairingsFile)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func ensureLocalToken(path string) (string, error) {
	if raw, err := os.ReadFile(path); err == nil {
		if t := strings.TrimSpace(string(raw)); Validate(t) == nil {
			return t, nil
		}
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	t := hex.EncodeToString(buf)
	if err := os.WriteFile(path, []byte(t), 0o600); err != nil {
		return "", err
	}
	return t, nil
}

func hash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
