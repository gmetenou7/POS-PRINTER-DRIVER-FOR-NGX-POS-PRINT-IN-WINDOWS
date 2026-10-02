package api

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/gmetenou7/print-bridge/internal/config"
	"github.com/gmetenou7/print-bridge/internal/pairing"
)

// originSet est la liste des origines autorisees, sous la forme qu'envoie un navigateur.
// L'entree "*" autorise toute origine : c'est un choix explicite de config.json, jamais un defaut.
type originSet struct {
	any bool
	set map[string]bool
}

func newOriginSet(list []string) originSet {
	o := originSet{set: map[string]bool{}}
	for _, v := range config.NormalizeOrigins(list) {
		if v == "*" {
			o.any = true
			continue
		}
		o.set[v] = true
	}
	return o
}

func (o originSet) allows(origin string) bool {
	if origin == "" {
		return false
	}
	return o.any || o.set[config.NormalizeOrigin(origin)]
}

// withOriginGuard applique la liste des origines, cote navigateur (CORS) et cote agent.
//
// Le CORS seul ne protege rien ici : il empeche une page de LIRE la reponse, pas d'envoyer la
// requete. Un POST /print parti d'un site quelconque imprimerait, ou ouvrirait le tiroir, meme
// si la page ne voyait jamais le resultat. L'agent refuse donc lui-meme, avant tout effet, une
// requete dont l'en-tete Origin n'est pas dans la liste. Un navigateur envoie toujours cet
// en-tete sur un appel d'une autre origine qui n'est pas un simple GET, et une page ne peut ni
// le retirer ni le falsifier.
//
// Sans en-tete Origin (barre de notification, outil en ligne de commande), la requete passe
// cette garde : c'est le jeton qui decide ensuite.
//
// /health reste joignable par tous en GET, sans en-tetes CORS pour une origine refusee : le
// navigateur en masque alors la reponse, qui ne contient de toute facon rien de sensible.
func (s *Server) withOriginGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		allowed := s.origins.allows(origin)
		w.Header().Add("Vary", "Origin")

		if allowed {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, "+pairing.Header)
			w.Header().Set("Access-Control-Max-Age", "600")

			// Acces au reseau local (Private Network Access).
			//
			// Une page servie depuis un site public qui appelle une adresse locale, 127.0.0.1 ou
			// localhost, declenche chez Chrome un prevol portant l'en-tete
			// Access-Control-Request-Private-Network. Si la reponse ne l'autorise pas
			// explicitement, la requete est refusee, et la page ne voit qu'un echec reseau sans
			// cause lisible. Le cas s'observe comme « aucune imprimante pilotee par ce poste »
			// alors que l'agent tourne.
			//
			// Cette autorisation n'est donnee qu'aux origines de la liste : pour les autres, le
			// refus de Chrome est une protection de plus.
			if r.Header.Get("Access-Control-Request-Private-Network") == "true" {
				w.Header().Set("Access-Control-Allow-Private-Network", "true")
			}
		}

		if origin != "" && !allowed {
			if r.Method == http.MethodGet && r.URL.Path == "/health" {
				next.ServeHTTP(w, r)
				return
			}
			writeErr(w, http.StatusForbidden, "origin_not_allowed")
			return
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requireToken n'ouvre une route qu'a un jeton associe (ou au jeton local).
//
// La reponse 401 porte toujours `pairing_required` : le client sait qu'il doit s'associer, et le
// distingue d'un agent absent.
func (s *Server) requireToken(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.pairs.Valid(r.Header.Get(pairing.Header)) {
			writeErr(w, http.StatusUnauthorized, "pairing_required")
			return
		}
		next(w, r)
	}
}

type pairRequest struct {
	Token string `json:"token"`
}

// handlePair associe (POST) ou dissocie (DELETE) le jeton d'une application.
//
// L'association n'est acceptee que d'une origine autorisee, en-tete Origin present : c'est ce
// qui empeche un site quelconque de s'associer lui-meme. La dissociation exige le jeton, en
// en-tete ou dans le corps.
func (s *Server) handlePair(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		origin := r.Header.Get("Origin")
		if !s.origins.allows(origin) {
			writeErr(w, http.StatusForbidden, "origin_not_allowed")
			return
		}
		var req pairRequest
		if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&req); err != nil {
			writeErr(w, http.StatusBadRequest, "JSON invalide : "+err.Error())
			return
		}
		if err := s.pairs.Pair(req.Token, config.NormalizeOrigin(origin)); err != nil {
			code := http.StatusInternalServerError
			if err == pairing.ErrInvalidToken {
				code = http.StatusBadRequest
			}
			writeErr(w, code, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "paired": true})
	case http.MethodDelete:
		token := r.Header.Get(pairing.Header)
		if token == "" {
			var req pairRequest
			_ = json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&req)
			token = req.Token
		}
		if token == "" || s.pairs.IsLocal(token) {
			writeErr(w, http.StatusUnauthorized, "pairing_required")
			return
		}
		removed, err := s.pairs.Unpair(token)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !removed {
			writeErr(w, http.StatusUnauthorized, "pairing_required")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "paired": false})
	default:
		writeErr(w, http.StatusMethodNotAllowed, "POST ou DELETE")
	}
}
