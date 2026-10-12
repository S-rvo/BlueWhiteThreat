package scraper_tests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/S-rvo/BlueWhiteThreat/apps/scraper/internal/sites"
)

func writeTempSitesConfig(t *testing.T, content string) string {
	dir := t.TempDir()
	path := filepath.Join(dir, "sites.json")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}
	return path
}

func TestLoadSiteConfigs_ParsesKnownDomain(t *testing.T) {
	path := writeTempSitesConfig(t, `{
		"exampleforum.onion": {
			"login_url": "http://exampleforum.onion/login",
			"username_selector": "#username",
			"password_selector": "#password",
			"submit_selector": "button[type=submit]",
			"username_env": "SITE_EXAMPLEFORUM_USER",
			"password_env": "SITE_EXAMPLEFORUM_PASS"
		}
	}`)

	configs, err := sites.LoadSiteConfigs(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg, ok := configs["exampleforum.onion"]
	if !ok {
		t.Fatal("expected domain not found in configs")
	}
	if cfg.LoginURL != "http://exampleforum.onion/login" {
		t.Errorf("got LoginURL=%q", cfg.LoginURL)
	}
	if cfg.UsernameEnv != "SITE_EXAMPLEFORUM_USER" {
		t.Errorf("got UsernameEnv=%q", cfg.UsernameEnv)
	}
}

func TestLoadSiteConfigs_UnknownDomainAbsent(t *testing.T) {
	path := writeTempSitesConfig(t, `{}`)

	configs, err := sites.LoadSiteConfigs(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := configs["unknown.onion"]; ok {
		t.Error("expected unknown domain to be absent")
	}
}

func TestLoadSiteConfigs_MissingFileReturnsEmptyMap(t *testing.T) {
	configs, err := sites.LoadSiteConfigs("/path/does/not/exist.json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(configs) != 0 {
		t.Errorf("expected empty map, got %#v", configs)
	}
}
