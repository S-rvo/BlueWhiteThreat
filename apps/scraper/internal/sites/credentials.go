package sites

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
)

// storedCredentials is an authenticated ciphertext bound to one origin.
// Values include login credentials and optional registration fields.
type storedCredentials struct {
	Origin     string `json:"origin"`
	Ciphertext string `json:"ciphertext"`
}

func siteOrigin(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return "", errors.New("invalid credential origin")
	}
	return u.Scheme + "://" + u.Host, nil
}

func credentialCipher(keyEnv string) (cipher.AEAD, error) {
	key, err := base64.StdEncoding.DecodeString(envOrEmpty(keyEnv))
	if err != nil || len(key) != 32 {
		return nil, errors.New("credential key must be base64 of 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func sealCredentials(origin, keyEnv string, values map[string]string) (*storedCredentials, error) {
	aead, err := credentialCipher(keyEnv)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(values)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	sealed := aead.Seal(nonce, nonce, data, []byte(origin))
	return &storedCredentials{Origin: origin, Ciphertext: base64.StdEncoding.EncodeToString(sealed)}, nil
}

func openCredentials(origin, keyEnv string, stored *storedCredentials) (map[string]string, error) {
	if stored.Origin != origin {
		return nil, errors.New("credentials belong to another site")
	}
	aead, err := credentialCipher(keyEnv)
	if err != nil {
		return nil, err
	}
	data, err := base64.StdEncoding.DecodeString(stored.Ciphertext)
	if err != nil || len(data) < aead.NonceSize()+aead.Overhead() {
		return nil, errors.New("invalid credential ciphertext")
	}
	plain, err := aead.Open(nil, data[:aead.NonceSize()], data[aead.NonceSize():], []byte(origin))
	if err != nil {
		return nil, errors.New("credential decryption failed")
	}
	var values map[string]string
	if err := json.Unmarshal(plain, &values); err != nil {
		return nil, errors.New("invalid credential payload")
	}
	return values, nil
}

func environmentCredentials(cfg *SiteConfig) map[string]string {
	values := map[string]string{}
	for _, name := range []string{cfg.UsernameEnv, cfg.PasswordEnv} {
		if name != "" {
			values[name] = envOrEmpty(name)
		}
	}
	if cfg.Registration != nil {
		for _, name := range cfg.Registration.Fields {
			if name != "" {
				values[name] = envOrEmpty(name)
			}
		}
	}
	return values
}

// Stored values win over environment changes: an existing account is never
// silently replaced by a different identity on restart.
func resolveCredentials(cfg *SiteConfig, state *accessState) (map[string]string, error) {
	origin, err := siteOrigin(cfg.LoginURL)
	if err != nil {
		return nil, err
	}
	if state.Origin != "" && state.Origin != origin {
		return nil, errors.New("state_file belongs to another site")
	}
	state.Origin = origin
	if state.Credentials != nil {
		if cfg.CredentialsKeyEnv == "" {
			return nil, errors.New("credentials_key_env required to read stored account")
		}
		return openCredentials(origin, cfg.CredentialsKeyEnv, state.Credentials)
	}
	return environmentCredentials(cfg), nil
}

func persistCredentials(cfg *SiteConfig, state *accessState, values map[string]string) error {
	if cfg.CredentialsKeyEnv == "" {
		return nil
	}
	if cfg.StateFile == "" {
		return errors.New("state_file required for credential persistence")
	}
	sealed, err := sealCredentials(state.Origin, cfg.CredentialsKeyEnv, values)
	if err != nil {
		return err
	}
	state.Credentials = sealed
	return writeState(cfg.StateFile, *state)
}
