package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Ports par defaut, et leurs replis quand ils sont deja pris par un autre programme.
//
// Les clients (ngx-pos-print, la barre de notification, le SDK) sondent ces quatre adresses dans
// cet ordre : 19101, 19100, 19103, 19102. Le repli ne vaut que pour le port par defaut ; un port
// choisi explicitement est un choix, on ne le remplace pas en silence.
const (
	DefaultPort       = 19100
	DefaultHTTPSPort  = 19101
	FallbackPort      = 19102
	FallbackHTTPSPort = 19103
	configFileName    = "config.json"
)

// DefaultAllowedOrigins sont les seules pages web autorisees a parler a l'agent.
//
// Une page ouverte sur le poste de caisse, n'importe laquelle, peut appeler 127.0.0.1 : sans
// cette liste, un site quelconque ouvrirait le tiroir-caisse ou imprimerait de faux tickets.
// L'application MonGerant (navigateur comme application Windows, qui charge la meme adresse)
// et le serveur de developpement sont autorises ; toute autre origine s'ajoute dans
// config.json ou par `-origins`.
var DefaultAllowedOrigins = []string{
	"https://app.ventegrid.com",
	"http://localhost:4200",
}

type Config struct {
	Port      int
	HTTPSPort int
	// Vrai quand le port vient du defaut : on peut alors se replier sur 19102 / 19103.
	PortFallback      bool
	HTTPSPortFallback bool
	BindAddr          string
	AllowedOrigins    []string
	DetectInterval    int
	LogToFile         bool
	LogPath           string
	CertDir           string
	// Dossier des reglages (config.json), des associations et du jeton local.
	DataDir string
}

func Default() *Config {
	return &Config{
		Port:              DefaultPort,
		HTTPSPort:         DefaultHTTPSPort,
		PortFallback:      true,
		HTTPSPortFallback: true,
		BindAddr:          "127.0.0.1",
		AllowedOrigins:    append([]string(nil), DefaultAllowedOrigins...),
		DetectInterval:    30,
		LogToFile:         true,
		LogPath:           defaultLogPath(),
		CertDir:           defaultCertDir(),
		DataDir:           DefaultDataDir(),
	}
}

// UseDataDir loge journal, certificats, reglages et associations dans dir.
func (c *Config) UseDataDir(dir string) {
	c.DataDir = dir
	c.LogPath = filepath.Join(dir, "agent.log")
	c.CertDir = filepath.Join(dir, "certs")
}

// FileSettings est le contenu de config.json, dans le dossier de l'agent. Un champ absent garde
// sa valeur par defaut.
type FileSettings struct {
	AllowedOrigins []string `json:"allowedOrigins,omitempty"`
	Port           int      `json:"port,omitempty"`
	// -1 coupe le HTTPS.
	HTTPSPort int `json:"httpsPort,omitempty"`
}

// SettingsPath est le chemin de config.json.
func (c *Config) SettingsPath() string {
	return filepath.Join(c.DataDir, configFileName)
}

// ReadSettings lit config.json ; un fichier absent rend des reglages vides, sans erreur.
func ReadSettings(path string) (FileSettings, error) {
	var s FileSettings
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		return s, fmt.Errorf("%s illisible : %w", path, err)
	}
	return s, nil
}

// WriteSettings ecrit config.json, en creant le dossier au besoin.
func WriteSettings(path string, s FileSettings) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o644)
}

// LoadFile applique config.json par-dessus les valeurs par defaut.
func (c *Config) LoadFile() error {
	s, err := ReadSettings(c.SettingsPath())
	if err != nil {
		return err
	}
	c.Apply(s)
	return nil
}

// Apply reporte les champs renseignes de s.
func (c *Config) Apply(s FileSettings) {
	if len(s.AllowedOrigins) > 0 {
		c.AllowedOrigins = NormalizeOrigins(s.AllowedOrigins)
	}
	if s.Port > 0 {
		c.Port = s.Port
		c.PortFallback = false
	}
	if s.HTTPSPort > 0 {
		c.HTTPSPort = s.HTTPSPort
		c.HTTPSPortFallback = false
	} else if s.HTTPSPort < 0 {
		c.HTTPSPort = 0
	}
}

// ParseOrigins decoupe une liste separee par des virgules, telle que `-origins` la recoit.
func ParseOrigins(list string) []string {
	return NormalizeOrigins(strings.Split(list, ","))
}

// NormalizeOrigins met chaque origine sous la forme qu'envoie un navigateur : schema et hote en
// minuscules, sans barre finale. Les entrees vides sont retirees.
func NormalizeOrigins(in []string) []string {
	out := make([]string, 0, len(in))
	for _, o := range in {
		if o = NormalizeOrigin(o); o != "" {
			out = append(out, o)
		}
	}
	return out
}

// NormalizeOrigin rend une origine comparable a l'en-tete Origin.
func NormalizeOrigin(o string) string {
	return strings.TrimRight(strings.ToLower(strings.TrimSpace(o)), "/")
}
