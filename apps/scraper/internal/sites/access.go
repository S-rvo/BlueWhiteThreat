package sites

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/storage"
	"github.com/chromedp/chromedp"
)

// RegistrationConfig uses stable credentials supplied through environment variables.
// An attempt is persisted before submitting, including on ambiguous network failures.
type RegistrationConfig struct {
	URL             string            `json:"url"`
	Fields          map[string]string `json:"fields"` // CSS selector -> environment variable
	SubmitSelector  string            `json:"submit_selector"`
	SuccessSelector string            `json:"success_selector"`
}

// CAPTCHAConfig connects an image/text challenge to an operator-configured solver.
// The solver receives only the selected challenge HTML, never the whole page.
type CAPTCHAConfig struct {
	Selector       string `json:"selector"`
	AnswerSelector string `json:"answer_selector"`
	SubmitSelector string `json:"submit_selector"`
	SolverURL      string `json:"solver_url"`
	TokenEnv       string `json:"token_env"`
}

type accessState struct {
	Origin                string                 `json:"origin,omitempty"`
	Credentials           *storedCredentials     `json:"credentials,omitempty"`
	LastLoginAt           *time.Time             `json:"last_login_at,omitempty"`
	RegistrationAttempted bool                   `json:"registration_attempted"`
	Cookies               []*network.CookieParam `json:"cookies,omitempty"`
}

var accessLocks sync.Map
var ErrHumanActionRequired = errors.New("human_action_required")

func readState(path string) (accessState, error) {
	var state accessState
	if path == "" {
		return state, nil
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	err = json.Unmarshal(data, &state)
	return state, err
}

func writeState(path string, state accessState) error {
	if path == "" {
		return errors.New("state_file required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".session-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

func sameOrigin(target, candidate string) bool {
	a, e1 := url.Parse(target)
	b, e2 := url.Parse(candidate)
	return e1 == nil && e2 == nil && a.Host != "" && a.User == nil && b.User == nil &&
		(a.Scheme == "http" || a.Scheme == "https") && a.Scheme == b.Scheme && a.Host == b.Host
}

func validateAccess(target string, cfg *SiteConfig) error {
	return validateAccessValues(target, cfg, environmentCredentials(cfg), true)
}

func validateAccessValues(target string, cfg *SiteConfig, values map[string]string, registration bool) error {
	if !sameOrigin(target, cfg.LoginURL) {
		return errors.New("login_url must share target origin")
	}
	if cfg.UsernameSelector == "" || cfg.PasswordSelector == "" || cfg.SubmitSelector == "" || cfg.SuccessSelector == "" {
		return errors.New("login selectors and success_selector required")
	}
	if values[cfg.UsernameEnv] == "" || values[cfg.PasswordEnv] == "" {
		return errors.New("missing credential environment variables")
	}
	if r := cfg.Registration; r != nil && registration {
		if cfg.StateFile == "" || !sameOrigin(target, r.URL) || r.SubmitSelector == "" || r.SuccessSelector == "" || len(r.Fields) == 0 {
			return errors.New("registration requires state_file, same-origin URL, fields and selectors")
		}
		for selector, variable := range r.Fields {
			if selector == "" || values[variable] == "" {
				return errors.New("missing registration field or environment variable")
			}
		}
	}
	return nil
}

func prepareAccess(ctx context.Context, target string, cfg *SiteConfig) (func(), error) {
	key := cfg.StateFile
	if key == "" {
		key = cfg.LoginURL
	}
	value, _ := accessLocks.LoadOrStore(key, make(chan struct{}, 1))
	lock := value.(chan struct{})
	select {
	case lock <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	release := func() { <-lock }
	fail := func(err error) (func(), error) { release(); return nil, err }
	state, err := readState(cfg.StateFile)
	if err != nil {
		return fail(err)
	}
	values, err := resolveCredentials(cfg, &state)
	if err != nil {
		return fail(err)
	}
	if err := validateAccessValues(target, cfg, values, !state.RegistrationAttempted); err != nil {
		return fail(err)
	}
	// Save before submitting a registration, so an interrupted run keeps its identity.
	if err := persistCredentials(cfg, &state, values); err != nil {
		return fail(err)
	}
	if len(state.Cookies) > 0 {
		err = chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error { return storage.SetCookies(state.Cookies).Do(ctx) }), chromedp.Navigate(target))
		if err != nil {
			return fail(err)
		}
		var found bool
		if err := selectorPresent(ctx, cfg.SuccessSelector, &found); err != nil {
			return fail(err)
		}
		if found {
			return release, nil
		}
	}
	if r := cfg.Registration; r != nil && !state.RegistrationAttempted {
		// Persist before any network operation: never create repeatedly after a timeout.
		state.RegistrationAttempted = true
		if err := writeState(cfg.StateFile, state); err != nil {
			return fail(err)
		}
		if err := chromedp.Run(ctx, chromedp.Navigate(r.URL)); err != nil {
			return fail(err)
		}
		for selector, variable := range r.Fields {
			if err := chromedp.Run(ctx, chromedp.WaitVisible(selector, chromedp.ByQuery), chromedp.SetValue(selector, values[variable], chromedp.ByQuery)); err != nil {
				return fail(err)
			}
		}
		if err := checkChallenge(ctx, cfg.CAPTCHA); err != nil {
			return fail(err)
		}
		if err := chromedp.Run(ctx, chromedp.Click(r.SubmitSelector, chromedp.ByQuery), chromedp.WaitVisible(r.SuccessSelector, chromedp.ByQuery)); err != nil {
			return fail(fmt.Errorf("registration confirmation pending: %w", err))
		}
	}
	if err := chromedp.Run(ctx, chromedp.Navigate(cfg.LoginURL), chromedp.WaitVisible(cfg.UsernameSelector, chromedp.ByQuery), chromedp.SetValue(cfg.UsernameSelector, values[cfg.UsernameEnv], chromedp.ByQuery), chromedp.SetValue(cfg.PasswordSelector, values[cfg.PasswordEnv], chromedp.ByQuery)); err != nil {
		return fail(err)
	}
	if err := checkChallenge(ctx, cfg.CAPTCHA); err != nil {
		return fail(err)
	}
	if err := chromedp.Run(ctx, chromedp.Click(cfg.SubmitSelector, chromedp.ByQuery), chromedp.WaitVisible(cfg.SuccessSelector, chromedp.ByQuery)); err != nil {
		return fail(fmt.Errorf("login confirmation pending: %w", err))
	}
	if cfg.StateFile != "" {
		now := time.Now().UTC()
		state.LastLoginAt = &now
		if err := writeState(cfg.StateFile, state); err != nil {
			return fail(err)
		}
	}
	return release, nil
}

func selectorPresent(ctx context.Context, selector string, present *bool) error {
	encoded, _ := json.Marshal(selector)
	return chromedp.Run(ctx, chromedp.Evaluate(`document.querySelector(`+string(encoded)+`) !== null`, present))
}

func checkChallenge(ctx context.Context, cfg *CAPTCHAConfig) error {
	if cfg == nil || cfg.Selector == "" {
		return nil
	}
	var present bool
	if err := selectorPresent(ctx, cfg.Selector, &present); err != nil {
		return err
	}
	if !present {
		return nil
	}
	if cfg.SolverURL == "" || cfg.AnswerSelector == "" {
		return ErrHumanActionRequired
	}
	var challenge string
	if err := chromedp.Run(ctx, chromedp.OuterHTML(cfg.Selector, &challenge, chromedp.ByQuery)); err != nil {
		return err
	}
	answer, err := solveChallenge(ctx, cfg, challenge)
	if err != nil {
		return err
	}
	if err := chromedp.Run(ctx, chromedp.SetValue(cfg.AnswerSelector, answer, chromedp.ByQuery)); err != nil {
		return err
	}
	if cfg.SubmitSelector != "" {
		return chromedp.Run(ctx, chromedp.Click(cfg.SubmitSelector, chromedp.ByQuery), chromedp.WaitNotPresent(cfg.Selector, chromedp.ByQuery))
	}
	return nil
}

func solveChallenge(ctx context.Context, cfg *CAPTCHAConfig, challenge string) (string, error) {
	u, err := url.Parse(cfg.SolverURL)
	if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1"))) {
		return "", errors.New("solver requires HTTPS or loopback HTTP")
	}
	if len(challenge) > 256*1024 {
		return "", errors.New("challenge too large")
	}
	data, _ := json.Marshal(map[string]string{"challenge_html": challenge})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.SolverURL, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.TokenEnv != "" {
		token := envOrEmpty(cfg.TokenEnv)
		if token == "" {
			return "", errors.New("missing solver token")
		}
		req.Header.Set("Authorization", "Bearer "+token)
	}
	client := &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return "", errors.New("solver request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("solver status %d", resp.StatusCode)
	}
	var result struct {
		Answer string `json:"answer"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 16384)).Decode(&result); err != nil {
		return "", errors.New("invalid solver response")
	}
	if result.Answer == "" {
		return "", ErrHumanActionRequired
	}
	return result.Answer, nil
}

func saveSession(ctx context.Context, path string) error {
	if path == "" {
		return nil
	}
	state, err := readState(path)
	if err != nil {
		return err
	}
	err = chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		cookies, err := storage.GetCookies().Do(ctx)
		if err != nil {
			return err
		}
		state.Cookies = nil
		for _, cookie := range cookies {
			data, err := json.Marshal(cookie)
			if err != nil {
				return err
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(data, &fields); err != nil {
				return err
			}
			if cookie.Session {
				delete(fields, "expires")
			}
			data, err = json.Marshal(fields)
			if err != nil {
				return err
			}
			var param network.CookieParam
			if err := json.Unmarshal(data, &param); err != nil {
				return err
			}
			state.Cookies = append(state.Cookies, &param)
		}
		return nil
	}))
	if err != nil {
		return err
	}
	return writeState(path, state)
}
