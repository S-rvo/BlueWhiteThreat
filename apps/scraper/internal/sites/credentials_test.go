package sites

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func TestCredentialPersistenceAndReuse(t *testing.T) {
	t.Setenv("TEST_ACCOUNT_KEY", base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{42}, 32)))
	t.Setenv("TEST_ACCOUNT_USER", "alice")
	t.Setenv("TEST_ACCOUNT_PASS", "very-private-password")
	cfg := &SiteConfig{LoginURL: "https://example.test/login", UsernameEnv: "TEST_ACCOUNT_USER", PasswordEnv: "TEST_ACCOUNT_PASS", CredentialsKeyEnv: "TEST_ACCOUNT_KEY", StateFile: filepath.Join(t.TempDir(), "site.json")}
	var state accessState
	values, err := resolveCredentials(cfg, &state)
	if err != nil {
		t.Fatal(err)
	}
	if err := persistCredentials(cfg, &state, values); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(cfg.StateFile)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("very-private-password")) || bytes.Contains(raw, []byte("alice")) {
		t.Fatal("plaintext credentials on disk")
	}
	// Simulate restart without the original environment secrets.
	t.Setenv("TEST_ACCOUNT_USER", "")
	t.Setenv("TEST_ACCOUNT_PASS", "")
	restored, err := readState(cfg.StateFile)
	if err != nil {
		t.Fatal(err)
	}
	reused, err := resolveCredentials(cfg, &restored)
	if err != nil || reused[cfg.UsernameEnv] != "alice" || reused[cfg.PasswordEnv] != "very-private-password" {
		t.Fatal("saved account not restored", err)
	}
	cfg.LoginURL = "https://other.test/login"
	if _, err := resolveCredentials(cfg, &restored); err == nil {
		t.Fatal("cross-site credentials accepted")
	}
}

func TestCredentialAuthentication(t *testing.T) {
	t.Setenv("KEY", base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32)))
	stored, err := sealCredentials("https://example.test", "KEY", map[string]string{"user": "alice"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := openCredentials("https://other.test", "KEY", stored); err == nil {
		t.Fatal("wrong origin accepted")
	}
	// Changing metadata alone must not defeat the authenticated origin binding.
	stored.Origin = "https://other.test"
	if _, err := openCredentials(stored.Origin, "KEY", stored); err == nil {
		t.Fatal("modified associated data accepted")
	}
	stored.Origin = "https://example.test"
	t.Setenv("KEY", base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{2}, 32)))
	if _, err := openCredentials(stored.Origin, "KEY", stored); err == nil {
		t.Fatal("wrong key accepted")
	}
	t.Setenv("KEY", "invalid")
	if _, err := credentialCipher("KEY"); err == nil {
		t.Fatal("invalid key accepted")
	}
}

func TestStoredAccountSkipsRegistrationFieldsValidation(t *testing.T) {
	cfg := &SiteConfig{LoginURL: "https://example.test/login", UsernameEnv: "USER", PasswordEnv: "PASS", UsernameSelector: "#user", PasswordSelector: "#pass", SubmitSelector: "button", SuccessSelector: "#logout", Registration: &RegistrationConfig{URL: "https://example.test/register", Fields: map[string]string{"#email": "EMAIL"}}}
	values := map[string]string{"USER": "alice", "PASS": "secret"}
	if err := validateAccessValues("https://example.test/page", cfg, values, false); err != nil {
		t.Fatal(err)
	}
	if err := validateAccessValues("https://example.test/page", cfg, values, true); err == nil {
		t.Fatal("incomplete registration accepted")
	}
}
