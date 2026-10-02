package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gmetenou7/print-bridge/internal/config"
	"github.com/gmetenou7/print-bridge/internal/pairing"
	"github.com/gmetenou7/print-bridge/internal/printers"
)

const (
	appOrigin  = "https://app.ventegrid.com"
	evilOrigin = "https://evil.example"
	token      = "0123456789abcdef0123456789abcdef-jeton-de-test"
)

type fixture struct {
	srv    *Server
	pairs  *pairing.Store
	prints atomic.Int32
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	cfg := config.Default()
	cfg.UseDataDir(t.TempDir())
	pairs, err := pairing.Open(cfg.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	reg := printers.NewRegistry()
	reg.Upsert(printers.Printer{ID: "p1", Name: "Ticket", Channel: printers.ChannelNetwork, IsThermal: true, IsDefault: true})
	f := &fixture{pairs: pairs}
	f.srv = NewServer(cfg, reg, func(p printers.Printer, data []byte) (int, error) {
		f.prints.Add(1)
		return len(data), nil
	}, pairs)
	return f
}

func (f *fixture) do(method, path, origin, tok, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	if tok != "" {
		req.Header.Set(pairing.Header, tok)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	f.srv.Handler().ServeHTTP(rec, req)
	return rec
}

func errorOf(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return body.Error
}

const printBody = `{"text":"bonjour","openDrawer":true}`

func TestCORSEchoesOnlyAllowedOrigin(t *testing.T) {
	f := newFixture(t)

	pre := httptest.NewRequest(http.MethodOptions, "/print", nil)
	pre.Header.Set("Origin", appOrigin)
	pre.Header.Set("Access-Control-Request-Method", "POST")
	pre.Header.Set("Access-Control-Request-Private-Network", "true")
	rec := httptest.NewRecorder()
	f.srv.Handler().ServeHTTP(rec, pre)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("prevol autorise : code %d", rec.Code)
	}
	h := rec.Header()
	if got := h.Get("Access-Control-Allow-Origin"); got != appOrigin {
		t.Fatalf("Allow-Origin = %q, attendu l'origine elle-meme", got)
	}
	if !strings.Contains(h.Get("Access-Control-Allow-Headers"), pairing.Header) {
		t.Fatalf("Allow-Headers sans l'en-tete du jeton : %q", h.Get("Access-Control-Allow-Headers"))
	}
	if h.Get("Access-Control-Allow-Private-Network") != "true" {
		t.Fatal("reseau local non autorise pour une origine de la liste")
	}
	if !strings.Contains(strings.Join(h.Values("Vary"), ","), "Origin") {
		t.Fatal("Vary: Origin manquant")
	}
}

func TestCORSDeniesOtherOrigin(t *testing.T) {
	f := newFixture(t)

	pre := httptest.NewRequest(http.MethodOptions, "/print", nil)
	pre.Header.Set("Origin", evilOrigin)
	pre.Header.Set("Access-Control-Request-Method", "POST")
	pre.Header.Set("Access-Control-Request-Private-Network", "true")
	rec := httptest.NewRecorder()
	f.srv.Handler().ServeHTTP(rec, pre)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("prevol d'une origine inconnue : code %d, attendu 403", rec.Code)
	}
	for _, name := range []string{"Access-Control-Allow-Origin", "Access-Control-Allow-Private-Network"} {
		if v := rec.Header().Get(name); v != "" {
			t.Fatalf("%s = %q pour une origine refusee", name, v)
		}
	}
}

func TestForeignOriginCannotPrintEvenWithToken(t *testing.T) {
	f := newFixture(t)
	if err := f.pairs.Pair(token, appOrigin); err != nil {
		t.Fatal(err)
	}
	// Le navigateur ne laisserait pas lire la reponse, mais l'impression aurait lieu : c'est
	// l'agent qui doit refuser avant tout effet.
	rec := f.do(http.MethodPost, "/print", evilOrigin, token, printBody)
	if rec.Code != http.StatusForbidden || errorOf(t, rec) != "origin_not_allowed" {
		t.Fatalf("code %d, erreur %q", rec.Code, errorOf(t, rec))
	}
	if n := f.prints.Load(); n != 0 {
		t.Fatalf("%d impression(s) parties d'une origine refusee", n)
	}
}

func TestHealthStaysOpenWithoutCORSForForeignOrigin(t *testing.T) {
	f := newFixture(t)
	rec := f.do(http.MethodGet, "/health", evilOrigin, "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("health : code %d", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("health lisible par une origine refusee")
	}
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["pairingRequired"] != true {
		t.Fatalf("pairingRequired absent : %v", body)
	}
	if _, ok := body["paired"]; ok {
		t.Fatal("paired rendu sans jeton")
	}
}

func TestTokenRequiredOnEveryFunctionalRoute(t *testing.T) {
	f := newFixture(t)
	routes := []struct{ method, path, body string }{
		{http.MethodGet, "/printers", ""},
		{http.MethodGet, "/printers/p1", ""},
		{http.MethodGet, "/printers/p1/capabilities", ""},
		{http.MethodPost, "/print", printBody},
		{http.MethodPost, "/print/text", "bonjour"},
		{http.MethodPost, "/print-document", `{"pages":["aGVsbG8="]}`},
	}
	for _, r := range routes {
		for _, tok := range []string{"", "un-jeton-jamais-associe-de-plus-de-32-caracteres"} {
			rec := f.do(r.method, r.path, appOrigin, tok, r.body)
			if rec.Code != http.StatusUnauthorized || errorOf(t, rec) != "pairing_required" {
				t.Fatalf("%s %s jeton %q : code %d, erreur %q", r.method, r.path, tok, rec.Code, errorOf(t, rec))
			}
		}
	}
	if n := f.prints.Load(); n != 0 {
		t.Fatalf("%d impression(s) sans jeton", n)
	}
}

func TestPairingFlow(t *testing.T) {
	f := newFixture(t)

	// Une origine inconnue ne peut pas s'associer elle-meme, ni sans en-tete Origin.
	for _, origin := range []string{evilOrigin, ""} {
		rec := f.do(http.MethodPost, "/pair", origin, "", `{"token":"`+token+`"}`)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("association depuis %q : code %d", origin, rec.Code)
		}
	}
	// Un jeton trop court est refuse.
	if rec := f.do(http.MethodPost, "/pair", appOrigin, "", `{"token":"court"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("jeton court : code %d", rec.Code)
	}

	if rec := f.do(http.MethodPost, "/pair", appOrigin, "", `{"token":"`+token+`"}`); rec.Code != http.StatusOK {
		t.Fatalf("association : code %d %s", rec.Code, rec.Body.String())
	}

	if rec := f.do(http.MethodGet, "/printers", appOrigin, token, ""); rec.Code != http.StatusOK {
		t.Fatalf("liste apres association : code %d", rec.Code)
	}
	if rec := f.do(http.MethodPost, "/print", appOrigin, token, printBody); rec.Code != http.StatusOK {
		t.Fatalf("impression apres association : code %d %s", rec.Code, rec.Body.String())
	}
	if n := f.prints.Load(); n != 1 {
		t.Fatalf("%d impression(s), attendu 1", n)
	}

	var health map[string]any
	_ = json.Unmarshal(f.do(http.MethodGet, "/health", appOrigin, token, "").Body.Bytes(), &health)
	if health["paired"] != true {
		t.Fatalf("health ne voit pas l'association : %v", health)
	}

	// L'association survit a un redemarrage : l'empreinte est sur disque, pas le jeton.
	again, err := pairing.Open(f.srv.cfg.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	if !again.Valid(token) {
		t.Fatal("association perdue au redemarrage")
	}
}

func TestSeveralTokensCoexist(t *testing.T) {
	f := newFixture(t)
	other := strings.Repeat("b", 40)
	for _, tok := range []string{token, other} {
		if rec := f.do(http.MethodPost, "/pair", appOrigin, "", `{"token":"`+tok+`"}`); rec.Code != http.StatusOK {
			t.Fatalf("association : code %d", rec.Code)
		}
	}
	for _, tok := range []string{token, other} {
		if rec := f.do(http.MethodGet, "/printers", appOrigin, tok, ""); rec.Code != http.StatusOK {
			t.Fatalf("jeton %q : code %d", tok, rec.Code)
		}
	}
}

func TestUnpair(t *testing.T) {
	f := newFixture(t)
	f.do(http.MethodPost, "/pair", appOrigin, "", `{"token":"`+token+`"}`)

	// Sans le jeton, pas de dissociation.
	if rec := f.do(http.MethodDelete, "/pair", appOrigin, "", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("dissociation sans jeton : code %d", rec.Code)
	}
	if rec := f.do(http.MethodDelete, "/pair", appOrigin, token, ""); rec.Code != http.StatusOK {
		t.Fatalf("dissociation : code %d", rec.Code)
	}
	rec := f.do(http.MethodPost, "/print", appOrigin, token, printBody)
	if rec.Code != http.StatusUnauthorized || errorOf(t, rec) != "pairing_required" {
		t.Fatalf("impression apres dissociation : code %d", rec.Code)
	}
	// Le jeton local ne se dissocie pas.
	if rec := f.do(http.MethodDelete, "/pair", "", f.pairs.LocalToken(), ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("dissociation du jeton local : code %d", rec.Code)
	}
}

func TestLocalTokenServesTheTray(t *testing.T) {
	f := newFixture(t)
	local := pairing.ReadLocalToken(f.srv.cfg.DataDir)
	if local == "" || local != f.pairs.LocalToken() {
		t.Fatal("jeton local absent du dossier de l'agent")
	}
	// La barre de notification n'envoie pas d'en-tete Origin.
	if rec := f.do(http.MethodPost, "/print", "", local, printBody); rec.Code != http.StatusOK {
		t.Fatalf("impression de test de la barre : code %d", rec.Code)
	}
}
