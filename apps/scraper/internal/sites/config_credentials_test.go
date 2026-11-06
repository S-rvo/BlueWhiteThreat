package sites

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfiguredCredentialKeyDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sites.json")
	data := `{"persistent.test":{"state_file":"/private/site.json"},"custom.test":{"state_file":"/private/custom.json","credentials_key_env":"CUSTOM_KEY"},"anonymous.test":{}}`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	configs, err := LoadSiteConfigs(path)
	if err != nil {
		t.Fatal(err)
	}
	if configs["persistent.test"].CredentialsKeyEnv != "BWT_CREDENTIALS_KEY" {
		t.Fatal("missing default key binding")
	}
	if configs["custom.test"].CredentialsKeyEnv != "CUSTOM_KEY" {
		t.Fatal("custom key overwritten")
	}
	if configs["anonymous.test"].CredentialsKeyEnv != "" {
		t.Fatal("persistence enabled without a state file")
	}
}
