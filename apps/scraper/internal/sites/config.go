package sites

import (
	"encoding/json"
	"os"
)

// SiteConfig décrit comment se connecter (optionnellement) à un site .onion
// avant d'en récupérer le contenu.
type SiteConfig struct {
	LoginURL         string `json:"login_url"`
	UsernameSelector string `json:"username_selector"`
	PasswordSelector string `json:"password_selector"`
	SubmitSelector   string `json:"submit_selector"`
	UsernameEnv      string `json:"username_env"`
	PasswordEnv      string `json:"password_env"`
}

// LoadSiteConfigs charge la config par domaine depuis un fichier JSON.
// Si le fichier n'existe pas, renvoie une map vide (aucun site configuré
// pour le login — comportement par défaut : scraping anonyme).
func LoadSiteConfigs(path string) (map[string]SiteConfig, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path is an operator-provided config location, not user input
	if os.IsNotExist(err) {
		return map[string]SiteConfig{}, nil
	}
	if err != nil {
		return nil, err
	}

	var configs map[string]SiteConfig
	if err := json.Unmarshal(data, &configs); err != nil {
		return nil, err
	}
	return configs, nil
}
