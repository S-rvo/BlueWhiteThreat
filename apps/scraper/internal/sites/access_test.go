package sites

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestAccessState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions", "site.json")
	state, err := readState(path)
	if err != nil || state.RegistrationAttempted {
		t.Fatal(state, err)
	}
	state.RegistrationAttempted = true
	if err := writeState(path, state); err != nil {
		t.Fatal(err)
	}
	got, err := readState(path)
	if err != nil || !got.RegistrationAttempted {
		t.Fatal(got, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatal(info.Mode())
	}
	if err := os.WriteFile(path, []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readState(path); err == nil {
		t.Fatal("corrupt state accepted")
	}
}

func TestAccessValidation(t *testing.T) {
	t.Setenv("TEST_USER", "user")
	t.Setenv("TEST_PASS", "secret")
	cfg := SiteConfig{LoginURL: "https://example.test/login", UsernameSelector: "#user", PasswordSelector: "#password", SubmitSelector: "button", SuccessSelector: "#logout", UsernameEnv: "TEST_USER", PasswordEnv: "TEST_PASS"}
	if err := validateAccess("https://example.test/page", &cfg); err != nil {
		t.Fatal(err)
	}
	if err := validateAccess("https://other.test/page", &cfg); err == nil {
		t.Fatal("cross origin accepted")
	}
	cfg.Registration = &RegistrationConfig{URL: "https://example.test/register"}
	if err := validateAccess("https://example.test/page", &cfg); err == nil {
		t.Fatal("incomplete registration accepted")
	}
}

func TestSolver(t *testing.T) {
	t.Setenv("TEST_SOLVER_TOKEN", "secret")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer secret" {
			t.Error("invalid solver request")
		}
		var request map[string]string
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		switch request["challenge_html"] {
		case "ok":
			w.Write([]byte(`{"answer":"1234"}`))
		case "empty":
			w.Write([]byte(`{"answer":""}`))
		default:
			w.WriteHeader(http.StatusTooManyRequests)
		}
	}))
	defer server.Close()
	cfg := &CAPTCHAConfig{SolverURL: server.URL, TokenEnv: "TEST_SOLVER_TOKEN"}
	answer, err := solveChallenge(context.Background(), cfg, "ok")
	if err != nil || answer != "1234" {
		t.Fatal(answer, err)
	}
	if _, err := solveChallenge(context.Background(), cfg, "empty"); !errors.Is(err, ErrHumanActionRequired) {
		t.Fatal(err)
	}
	if _, err := solveChallenge(context.Background(), cfg, "error"); err == nil {
		t.Fatal("status ignored")
	}
	cfg.SolverURL = "http://remote.test/solver"
	if _, err := solveChallenge(context.Background(), cfg, "ok"); err == nil {
		t.Fatal("insecure remote accepted")
	}
}
