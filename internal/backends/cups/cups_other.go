//go:build !windows

// Package cups pilote les imprimantes installees dans CUPS, le spouleur de Linux et de macOS.
//
// C'est l'equivalent de la voie GDI de Windows, et il est bien plus simple : CUPS accepte
// directement une image ou un PDF, applique lui-meme le pilote, et prend ses reglages en
// options de ligne de commande. Aucun appel systeme a ecrire, aucun DEVMODE a remplir.
//
// On parle a CUPS par ses commandes plutot que par sa bibliotheque C : cela garde le binaire
// unique, sans cgo, et ces commandes sont presentes partout ou CUPS l'est.
package cups

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gmetenou7/print-bridge/internal/printers"
)

// Toute commande est bornee dans le temps.
//
// Ce n'est pas une precaution de style. `lpoptions` interroge l'imprimante pour lire ce qu'elle
// sait faire, et sur une file dont l'appareil ne repond plus, il attend sans fin : une file
// « implicitclass » restee en place apres un debranchement suffit. L'agent bloquerait alors sur
// cette requete, puis sur toutes les suivantes.
const (
	queryTimeout = 6 * time.Second
	printTimeout = 60 * time.Second
)

// Info est une file d'impression declaree dans CUPS.
type Info struct {
	Name      string
	Status    string
	IsDefault bool
	Device    string
}

// Available dit si CUPS est joignable sur cette machine.
func Available() bool {
	_, err := exec.LookPath("lpstat")
	return err == nil
}

// List enumere les files d'impression et leur etat.
func List() ([]Info, error) {
	if !Available() {
		return nil, fmt.Errorf("cups absent : lpstat introuvable")
	}

	out, err := run(queryTimeout, "lpstat", "-p")
	if err != nil {
		// Aucune imprimante declaree : lpstat sort en erreur, ce qui n'est pas une panne.
		return nil, nil
	}

	devices := deviceURIs()
	def := defaultPrinter()

	var list []Info
	for _, line := range strings.Split(out, "\n") {
		// « printer NOM is idle.  enabled since ... » ou « printer NOM disabled since ... »
		if !strings.HasPrefix(line, "printer ") {
			continue
		}
		rest := strings.TrimPrefix(line, "printer ")
		name, state, found := strings.Cut(rest, " ")
		if !found || name == "" {
			continue
		}
		list = append(list, Info{
			Name:      name,
			Status:    translateState(state),
			IsDefault: name == def,
			Device:    devices[name],
		})
	}
	return list, nil
}

// Capabilities lit ce que le pilote de la file declare savoir faire.
//
// Les reglages de CUPS portent des mots-cles, « A4 », « DuplexNoTumble », la ou Windows porte
// des numeros. Le client, lui, ne manipule que des numeros. On rend donc le rang dans la liste,
// et c'est l'agent qui retrouve le mot-cle au moment d'imprimer : l'API reste la meme des deux
// cotes, et le client n'a pas a savoir sur quel systeme il imprime.
func Capabilities(name string) (printers.Caps, error) {
	// Listes vides, jamais nulles. Un encodage JSON rend une tranche nulle par `null`, et le
	// client qui compte ses elements se casse alors dessus : ce n'est pas a lui de se mefier
	// d'un contrat qui promet une liste.
	caps := printers.Caps{
		Papers:    []printers.NamedID{},
		Bins:      []printers.NamedID{},
		MaxCopies: 99,
		DPI:       300,
	}
	options, err := listOptions(name)
	if err != nil {
		return caps, err
	}

	for _, paper := range options["PageSize"] {
		caps.Papers = append(caps.Papers, printers.NamedID{ID: len(caps.Papers) + 1, Name: paper})
	}
	for _, bin := range options["InputSlot"] {
		caps.Bins = append(caps.Bins, printers.NamedID{ID: len(caps.Bins) + 1, Name: bin})
	}

	// Un pilote qui n'offre que « None » n'a pas de recto-verso.
	for _, mode := range options["Duplex"] {
		if !strings.EqualFold(mode, "None") {
			caps.Duplex = true
			break
		}
	}
	for _, mode := range append(options["ColorModel"], options["print-color-mode"]...) {
		if !isMonochrome(mode) {
			caps.Color = true
			break
		}
	}

	// Surface imprimable indicative, pour que l'appelant sache a quelle taille rendre ses
	// pages. CUPS ne l'annonce pas simplement, et de toute facon c'est lui qui ajuste a la
	// page : une A4 a 300 points par pouce fait l'affaire.
	caps.WidthPx = 2480
	caps.HeightPx = 3508
	return caps, nil
}

// PrintDocument imprime des pages deja rendues, sans aucune fenetre.
func PrintDocument(name, jobName string, pages [][]byte, opts printers.DocOptions) (int, error) {
	if len(pages) == 0 {
		return 0, fmt.Errorf("aucune page a imprimer")
	}
	dir, err := os.MkdirTemp("", "print-bridge-*")
	if err != nil {
		return 0, err
	}
	defer os.RemoveAll(dir)

	files := make([]string, 0, len(pages))
	for i, page := range pages {
		path := filepath.Join(dir, fmt.Sprintf("page-%03d.png", i+1))
		if err := os.WriteFile(path, page, 0o600); err != nil {
			return 0, err
		}
		files = append(files, path)
	}

	args := []string{"-d", name}
	if jobName != "" {
		args = append(args, "-t", jobName)
	}
	if opts.Copies > 1 {
		args = append(args, "-n", strconv.Itoa(opts.Copies))
	}
	args = append(args, optionArgs(name, opts)...)
	// Sans cela, une image part a sa taille en points et deborde de la feuille.
	args = append(args, "-o", "fit-to-page")
	args = append(args, files...)

	if _, err := run(printTimeout, "lp", args...); err != nil {
		return 0, err
	}
	return len(pages), nil
}

// PrintRaw envoie un flux d'octets a une file, sans que CUPS n'y touche.
//
// C'est ainsi qu'une thermique declaree dans CUPS recoit son ESC/POS : l'option « raw »
// court-circuite les filtres, qui prendraient ces octets pour un document a mettre en page.
func PrintRaw(name, jobName string, data []byte) (int, error) {
	args := []string{"-d", name, "-o", "raw"}
	if jobName != "" {
		args = append(args, "-t", jobName)
	}
	ctx, cancel := context.WithTimeout(context.Background(), printTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "lp", args...)
	cmd.Stdin = bytes.NewReader(data)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return 0, fmt.Errorf("lp : %v (%s)", err, strings.TrimSpace(stderr.String()))
	}
	return len(data), nil
}

// optionArgs traduit les choix de l'utilisateur en options de CUPS.
func optionArgs(name string, opts printers.DocOptions) []string {
	var args []string

	if opts.Color != nil {
		if *opts.Color {
			args = append(args, "-o", "print-color-mode=color")
		} else {
			args = append(args, "-o", "print-color-mode=monochrome")
		}
	}
	switch opts.Duplex {
	case "none":
		args = append(args, "-o", "sides=one-sided")
	case "long":
		args = append(args, "-o", "sides=two-sided-long-edge")
	case "short":
		args = append(args, "-o", "sides=two-sided-short-edge")
	}
	if opts.Landscape != nil && *opts.Landscape {
		args = append(args, "-o", "landscape")
	}

	// Le client renvoie le rang que Capabilities lui avait donne : on relit la meme liste pour
	// retrouver le mot-cle qui va avec.
	if opts.Paper > 0 || opts.Bin > 0 {
		options, err := listOptions(name)
		if err == nil {
			if keyword := nth(options["PageSize"], opts.Paper); keyword != "" {
				args = append(args, "-o", "media="+keyword)
			}
			if keyword := nth(options["InputSlot"], opts.Bin); keyword != "" {
				args = append(args, "-o", "InputSlot="+keyword)
			}
		}
	}
	return args
}

// Les reglages d'une imprimante ne changent pas en cours de journee, et chaque lecture coute un
// aller-retour vers la machine. On les garde donc un moment : sans ce cache, une impression les
// relit une seconde fois juste pour traduire un numero de format en mot-cle.
var (
	optionsMu    sync.Mutex
	optionsCache = map[string]cachedOptions{}
)

type cachedOptions struct {
	values map[string][]string
	readAt time.Time
}

const optionsTTL = 5 * time.Minute

// listOptions rend, par reglage, la liste des valeurs que le pilote accepte.
//
// La sortie de lpoptions donne une ligne par reglage :
// « PageSize/Media Size: *A4 Letter Legal ». L'etoile marque la valeur courante, elle ne fait
// pas partie du mot-cle.
func listOptions(name string) (map[string][]string, error) {
	optionsMu.Lock()
	cached, ok := optionsCache[name]
	optionsMu.Unlock()
	if ok && time.Since(cached.readAt) < optionsTTL {
		return cached.values, nil
	}

	out, err := run(queryTimeout, "lpoptions", "-p", name, "-l")
	if err != nil {
		return nil, err
	}
	options := map[string][]string{}
	for _, line := range strings.Split(out, "\n") {
		head, values, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		key, _, _ := strings.Cut(strings.TrimSpace(head), "/")
		for _, value := range strings.Fields(values) {
			options[key] = append(options[key], strings.TrimPrefix(value, "*"))
		}
	}

	optionsMu.Lock()
	optionsCache[name] = cachedOptions{values: options, readAt: time.Now()}
	optionsMu.Unlock()
	return options, nil
}

// isMonochrome reconnait les valeurs de couleur qui n'en sont pas.
//
// Les pilotes n'ont pas de vocabulaire commun : « Gray », « AutoGray », « KGray », ou le
// « monochrome » de la norme IPP designent tous du noir et blanc. Une liste de ce qui est
// monochrome vaut mieux qu'une liste de ce qui est couleur, ou chaque constructeur invente son
// nom : une valeur inconnue est alors tenue pour de la couleur, et l'option apparait, ce qui se
// corrige d'un clic. L'inverse masquerait la couleur d'une machine qui en a.
func isMonochrome(mode string) bool {
	switch strings.ToLower(mode) {
	case "gray", "grayscale", "autogray", "kgray", "black", "mono", "monochrome",
		"bi-level", "process-monochrome", "auto-monochrome":
		return true
	}
	return false
}

func nth(values []string, rank int) string {
	if rank >= 1 && rank <= len(values) {
		return values[rank-1]
	}
	return ""
}

func deviceURIs() map[string]string {
	devices := map[string]string{}
	out, err := run(queryTimeout, "lpstat", "-v")
	if err != nil {
		return devices
	}
	for _, line := range strings.Split(out, "\n") {
		// « device for NOM: ipp://... »
		rest, ok := strings.CutPrefix(line, "device for ")
		if !ok {
			continue
		}
		name, uri, found := strings.Cut(rest, ":")
		if found {
			devices[strings.TrimSpace(name)] = strings.TrimSpace(uri)
		}
	}
	return devices
}

func defaultPrinter() string {
	out, err := run(queryTimeout, "lpstat", "-d")
	if err != nil {
		return ""
	}
	_, name, found := strings.Cut(out, ":")
	if !found {
		return ""
	}
	return strings.TrimSpace(name)
}

func translateState(state string) string {
	switch {
	case strings.HasPrefix(state, "is idle"):
		return "ready"
	case strings.HasPrefix(state, "now printing"), strings.HasPrefix(state, "is printing"):
		return "printing"
	case strings.HasPrefix(state, "disabled"):
		return "paused"
	default:
		return "unknown"
	}
}

func run(timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("%s : l'imprimante n'a pas repondu en %s", name, timeout)
		}
		return "", fmt.Errorf("%s : %v (%s)", name, err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}
