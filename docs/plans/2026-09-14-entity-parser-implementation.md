# Service Parser (extraction d'entités CTI) Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Créer `apps/parser`, un service Go qui lit les articles bruts de MongoDB, en extrait les entités CTI (IOCs, CVEs, malware, acteurs, infrastructure) via un LLM local (Ollama), valide le résultat, escalade vers Claude en cas d'échec, puis écrit les entités dans MongoDB et Neo4j.

**Architecture:** Nouveau module Go indépendant (`apps/parser/`), même style que `apps/scraper` (ticker + polling). Une interface `Extractor` découple la logique métier du fournisseur LLM ; `OllamaExtractor` est le chemin par défaut (gratuit, local), `ClaudeExtractor` (tool-use forcé, un seul appel synchrone par document escaladé) est le filet de sécurité. Voir le design doc : `docs/plans/2026-09-14-entity-parser-design.md`.

**Tech Stack:** Go 1.24, `go.mongodb.org/mongo-driver`, `github.com/neo4j/neo4j-go-driver/v5`, `github.com/anthropics/anthropic-sdk-go` (fallback uniquement), Ollama (`http://localhost:11434`, modèle `qwen2.5:14b-instruct`), `github.com/joho/godotenv`.

**Note de scope (déviation mineure vs. design doc) :** le design doc mentionne la Batches API pour l'escalade Claude. Pour le MVP, l'escalade se fait en appel synchrone unitaire par document (volume attendu : quelques % du total, latence non critique car asynchrone dans le ticker). Le passage à la Batches API est un raffinement futur si le volume d'escalade dépasse ce qui est raisonnable en synchrone — pas nécessaire pour valider l'architecture.

---

## Task 0 : Scaffolding du module `apps/parser`

**Files:**
- Create: `apps/parser/go.mod`
- Create: `apps/parser/cmd/main.go` (placeholder minimal pour l'instant)
- Create: `apps/parser/.env.example`

**Step 1: Créer le module Go**

```bash
mkdir -p apps/parser/cmd apps/parser/internal
cd apps/parser
go mod init github.com/S-rvo/BlueWhiteThreat/apps/parser
```

**Step 2: Placeholder main.go**

```go
package main

import "log"

func main() {
    log.Println("parser: scaffold OK")
}
```

**Step 3: Vérifier que ça compile**

Run: `cd apps/parser && go run ./cmd`
Expected: affiche `parser: scaffold OK`

**Step 4: `.env.example`**

```
MONGO_HOST=localhost
MONGO_PORT=27017
MONGO_INITDB_ROOT_USERNAME=root
MONGO_INITDB_ROOT_PASSWORD=root
NEO4J_URI=bolt://localhost:7687
NEO4J_USER=neo4j
NEO4J_PASSWORD=password
OLLAMA_HOST=http://localhost:11434
OLLAMA_MODEL=qwen2.5:14b-instruct
PARSER_INTERVAL_MIN=10
ANTHROPIC_API_KEY=
```

**Step 5: Commit**

```bash
git add apps/parser/go.mod apps/parser/cmd/main.go apps/parser/.env.example
git commit -m "chore(parser): scaffold module apps/parser"
```

---

## Task 1 : Types `Entities` + validation regex

**Files:**
- Create: `apps/parser/internal/entities/entities.go`
- Create: `apps/parser/internal/entities/validate.go`
- Test: `apps/parser/internal/entities/validate_test.go`

**Step 1: Write the failing test**

```go
// apps/parser/internal/entities/validate_test.go
package entities

import "testing"

func TestValidateIOCs_FiltersInvalidFormats(t *testing.T) {
    in := Entities{}
    in.IOCs.IPs = []string{"192.168.1.1", "not-an-ip", "999.999.999.999"}
    in.IOCs.Hashes = []string{"d41d8cd98f00b204e9800998ecf8427e", "tooshort"}
    in.IOCs.Domains = []string{"example.onion", "bad domain with space"}
    in.CVEs = []string{"CVE-2024-12345", "CVE-bad"}

    out := ValidateAndClean(in)

    if len(out.IOCs.IPs) != 1 || out.IOCs.IPs[0] != "192.168.1.1" {
        t.Fatalf("expected only valid IP kept, got %v", out.IOCs.IPs)
    }
    if len(out.IOCs.Hashes) != 1 {
        t.Fatalf("expected only valid hash kept, got %v", out.IOCs.Hashes)
    }
    if len(out.IOCs.Domains) != 1 {
        t.Fatalf("expected only valid domain kept, got %v", out.IOCs.Domains)
    }
    if len(out.CVEs) != 1 || out.CVEs[0] != "CVE-2024-12345" {
        t.Fatalf("expected only valid CVE kept, got %v", out.CVEs)
    }
}

func TestIsEmpty(t *testing.T) {
    if !(Entities{}).IsEmpty() {
        t.Fatal("zero-value Entities should be empty")
    }
    e := Entities{}
    e.CVEs = []string{"CVE-2024-1"}
    if e.IsEmpty() {
        t.Fatal("Entities with a CVE should not be empty")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `cd apps/parser && go test ./internal/entities/... -run TestValidateIOCs_FiltersInvalidFormats -v`
Expected: FAIL (build error — `Entities`, `ValidateAndClean` not defined)

**Step 3: Write minimal implementation**

```go
// apps/parser/internal/entities/entities.go
package entities

type Entities struct {
    IOCs struct {
        IPs     []string `json:"ips" bson:"ips"`
        Hashes  []string `json:"hashes" bson:"hashes"`
        Domains []string `json:"domains" bson:"domains"`
        URLs    []string `json:"urls" bson:"urls"`
    } `json:"iocs" bson:"iocs"`
    CVEs           []string `json:"cves" bson:"cves"`
    Malware        []string `json:"malware" bson:"malware"`
    ThreatActors   []string `json:"threat_actors" bson:"threat_actors"`
    Infrastructure []string `json:"infrastructure" bson:"infrastructure"`
}

func (e Entities) IsEmpty() bool {
    return len(e.IOCs.IPs) == 0 && len(e.IOCs.Hashes) == 0 &&
        len(e.IOCs.Domains) == 0 && len(e.IOCs.URLs) == 0 &&
        len(e.CVEs) == 0 && len(e.Malware) == 0 &&
        len(e.ThreatActors) == 0 && len(e.Infrastructure) == 0
}
```

```go
// apps/parser/internal/entities/validate.go
package entities

import "regexp"

var (
    reIPv4    = regexp.MustCompile(`^(\d{1,3}\.){3}\d{1,3}$`)
    reHash    = regexp.MustCompile(`^[a-fA-F0-9]{32}$|^[a-fA-F0-9]{40}$|^[a-fA-F0-9]{64}$`)
    reDomain  = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)+$`)
    reCVE     = regexp.MustCompile(`^CVE-\d{4}-\d{4,7}$`)
)

func isValidIPv4(s string) bool {
    if !reIPv4.MatchString(s) {
        return false
    }
    parts := regexp.MustCompile(`\.`).Split(s, -1)
    for _, p := range parts {
        if len(p) > 1 && p[0] == '0' {
            return false
        }
        n := 0
        for _, c := range p {
            n = n*10 + int(c-'0')
        }
        if n > 255 {
            return false
        }
    }
    return true
}

func filter(items []string, valid func(string) bool) []string {
    out := make([]string, 0, len(items))
    for _, it := range items {
        if valid(it) {
            out = append(out, it)
        }
    }
    return out
}

// ValidateAndClean garde uniquement les entités dont le format est correct.
// Élimine les hallucinations de format produites par le LLM.
func ValidateAndClean(in Entities) Entities {
    out := in
    out.IOCs.IPs = filter(in.IOCs.IPs, isValidIPv4)
    out.IOCs.Hashes = filter(in.IOCs.Hashes, reHash.MatchString)
    out.IOCs.Domains = filter(in.IOCs.Domains, reDomain.MatchString)
    out.CVEs = filter(in.CVEs, reCVE.MatchString)
    // Malware / ThreatActors / Infrastructure : pas de format vérifiable,
    // on les garde tels quels (validation sémantique hors scope regex).
    return out
}
```

**Step 4: Run test to verify it passes**

Run: `cd apps/parser && go test ./internal/entities/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add apps/parser/internal/entities/
git commit -m "feat(parser): add Entities type and format validation"
```

---

## Task 2 : Interface `Extractor`

**Files:**
- Create: `apps/parser/internal/extractor/extractor.go`

**Step 1: Écrire l'interface (pas de test — pure déclaration de contrat)**

```go
package extractor

import (
    "context"

    "github.com/S-rvo/BlueWhiteThreat/apps/parser/internal/entities"
)

// Extractor transforme un texte brut en entités CTI structurées.
type Extractor interface {
    Extract(ctx context.Context, text string) (entities.Entities, error)
}
```

**Step 2: Commit**

```bash
git add apps/parser/internal/extractor/extractor.go
git commit -m "feat(parser): define Extractor interface"
```

---

## Task 3 : `OllamaExtractor`

**Files:**
- Create: `apps/parser/internal/extractor/ollama.go`
- Test: `apps/parser/internal/extractor/ollama_test.go`

Ollama expose `POST /api/chat` avec un champ `format` acceptant un JSON Schema (mode structuré, `stream: false` pour une réponse complète en un bloc). Le test mock le serveur HTTP — pas besoin d'un vrai Ollama qui tourne pour ce test unitaire.

**Step 1: Write the failing test**

```go
// apps/parser/internal/extractor/ollama_test.go
package extractor

import (
    "context"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
)

func TestOllamaExtractor_ParsesStructuredResponse(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.URL.Path != "/api/chat" {
            t.Fatalf("unexpected path: %s", r.URL.Path)
        }
        resp := map[string]any{
            "message": map[string]any{
                "role": "assistant",
                "content": `{"iocs":{"ips":["1.2.3.4"],"hashes":[],"domains":[],"urls":[]},` +
                    `"cves":["CVE-2024-1234"],"malware":["Emotet"],"threat_actors":[],"infrastructure":[]}`,
            },
        }
        json.NewEncoder(w).Encode(resp)
    }))
    defer srv.Close()

    ext := NewOllamaExtractor(srv.URL, "qwen2.5:14b-instruct")
    out, err := ext.Extract(context.Background(), "some scraped article text")
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if len(out.IOCs.IPs) != 1 || out.IOCs.IPs[0] != "1.2.3.4" {
        t.Fatalf("expected IP extracted, got %v", out.IOCs.IPs)
    }
    if len(out.Malware) != 1 || out.Malware[0] != "Emotet" {
        t.Fatalf("expected malware extracted, got %v", out.Malware)
    }
}

func TestOllamaExtractor_ErrorsOnInvalidJSON(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        json.NewEncoder(w).Encode(map[string]any{
            "message": map[string]any{"role": "assistant", "content": "not json"},
        })
    }))
    defer srv.Close()

    ext := NewOllamaExtractor(srv.URL, "qwen2.5:14b-instruct")
    _, err := ext.Extract(context.Background(), "text")
    if err == nil {
        t.Fatal("expected error on malformed model output")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `cd apps/parser && go test ./internal/extractor/... -run TestOllamaExtractor -v`
Expected: FAIL (build error — `NewOllamaExtractor` not defined)

**Step 3: Write minimal implementation**

```go
// apps/parser/internal/extractor/ollama.go
package extractor

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "time"

    "github.com/S-rvo/BlueWhiteThreat/apps/parser/internal/entities"
)

type OllamaExtractor struct {
    baseURL string
    model   string
    client  *http.Client
}

func NewOllamaExtractor(baseURL, model string) *OllamaExtractor {
    return &OllamaExtractor{
        baseURL: baseURL,
        model:   model,
        client:  &http.Client{Timeout: 60 * time.Second},
    }
}

const systemPrompt = `Tu es un extracteur d'entités CTI. Analyse le texte fourni et retourne
UNIQUEMENT un objet JSON respectant strictement ce schéma, sans aucun texte autour :
{"iocs":{"ips":[],"hashes":[],"domains":[],"urls":[]},"cves":[],"malware":[],
"threat_actors":[],"infrastructure":[]}
Si aucune entité d'une catégorie n'est trouvée, renvoie un tableau vide pour cette catégorie.`

var jsonSchema = map[string]any{
    "type": "object",
    "properties": map[string]any{
        "iocs": map[string]any{
            "type": "object",
            "properties": map[string]any{
                "ips":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
                "hashes":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
                "domains": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
                "urls":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
            },
        },
        "cves":            map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
        "malware":         map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
        "threat_actors":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
        "infrastructure":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
    },
}

func (o *OllamaExtractor) Extract(ctx context.Context, text string) (entities.Entities, error) {
    reqBody := map[string]any{
        "model": o.model,
        "messages": []map[string]string{
            {"role": "system", "content": systemPrompt},
            {"role": "user", "content": text},
        },
        "format": jsonSchema,
        "stream": false,
    }
    buf, err := json.Marshal(reqBody)
    if err != nil {
        return entities.Entities{}, fmt.Errorf("marshal request: %w", err)
    }

    req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/api/chat", bytes.NewReader(buf))
    if err != nil {
        return entities.Entities{}, fmt.Errorf("build request: %w", err)
    }
    req.Header.Set("Content-Type", "application/json")

    resp, err := o.client.Do(req)
    if err != nil {
        return entities.Entities{}, fmt.Errorf("ollama request failed: %w", err)
    }
    defer resp.Body.Close()

    var parsed struct {
        Message struct {
            Content string `json:"content"`
        } `json:"message"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
        return entities.Entities{}, fmt.Errorf("decode ollama response: %w", err)
    }

    var out entities.Entities
    if err := json.Unmarshal([]byte(parsed.Message.Content), &out); err != nil {
        return entities.Entities{}, fmt.Errorf("model output is not valid JSON: %w", err)
    }
    return out, nil
}
```

**Step 4: Run test to verify it passes**

Run: `cd apps/parser && go test ./internal/extractor/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add apps/parser/internal/extractor/ollama.go apps/parser/internal/extractor/ollama_test.go
git commit -m "feat(parser): add OllamaExtractor with structured JSON output"
```

---

## Task 4 : `ClaudeExtractor` (fallback)

**Files:**
- Modify: `apps/parser/go.mod` (ajout dépendance)
- Create: `apps/parser/internal/extractor/claude.go`
- Test: `apps/parser/internal/extractor/claude_test.go`

Utilise le pattern "tool-use forcé" documenté dans le skill `claude-api` (Go) plutôt que `output_config` (non documenté pour Go dans le skill à ce jour — évite de deviner une API non vérifiée) : un seul outil `record_entities` dont le schéma d'input est le même que `Entities`, et un system prompt qui impose de toujours l'appeler.

**Step 1: Ajouter la dépendance**

Run: `cd apps/parser && go get github.com/anthropics/anthropic-sdk-go`

**Step 2: Write the failing test**

Le test vérifie uniquement la logique de parsing du tool call (pas d'appel réseau réel — on ne peut pas mocker le SDK Anthropic facilement, donc on isole la fonction de parsing).

```go
// apps/parser/internal/extractor/claude_test.go
package extractor

import "testing"

func TestParseToolInput_ValidJSON(t *testing.T) {
    raw := []byte(`{"iocs":{"ips":["8.8.8.8"],"hashes":[],"domains":[],"urls":[]},
        "cves":[],"malware":["Cobalt Strike"],"threat_actors":[],"infrastructure":[]}`)
    out, err := parseToolInput(raw)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if len(out.Malware) != 1 || out.Malware[0] != "Cobalt Strike" {
        t.Fatalf("expected malware parsed, got %v", out.Malware)
    }
}

func TestParseToolInput_InvalidJSON(t *testing.T) {
    _, err := parseToolInput([]byte("not json"))
    if err == nil {
        t.Fatal("expected error on invalid JSON")
    }
}
```

**Step 3: Run test to verify it fails**

Run: `cd apps/parser && go test ./internal/extractor/... -run TestParseToolInput -v`
Expected: FAIL (build error — `parseToolInput` not defined)

**Step 4: Write minimal implementation**

```go
// apps/parser/internal/extractor/claude.go
package extractor

import (
    "context"
    "encoding/json"
    "fmt"

    "github.com/anthropics/anthropic-sdk-go"

    "github.com/S-rvo/BlueWhiteThreat/apps/parser/internal/entities"
)

type ClaudeExtractor struct {
    client anthropic.Client
    model  anthropic.Model
}

func NewClaudeExtractor() *ClaudeExtractor {
    return &ClaudeExtractor{
        client: anthropic.NewClient(),
        model:  anthropic.ModelClaudeHaiku4_5_20251001,
    }
}

const claudeSystemPrompt = `Tu es un extracteur d'entités CTI. Appelle TOUJOURS l'outil
record_entities avec les entités trouvées dans le texte fourni (IOCs, CVEs, malware,
acteurs/groupes, infrastructure). Utilise des tableaux vides pour les catégories sans résultat.`

var recordEntitiesTool = anthropic.ToolParam{
    Name:        "record_entities",
    Description: anthropic.String("Enregistre les entités CTI extraites du texte"),
    InputSchema: anthropic.ToolInputSchemaParam{
        Properties: map[string]any{
            "iocs": map[string]any{
                "type": "object",
                "properties": map[string]any{
                    "ips":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
                    "hashes":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
                    "domains": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
                    "urls":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
                },
            },
            "cves":           map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
            "malware":        map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
            "threat_actors":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
            "infrastructure": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
        },
    },
}

func parseToolInput(raw []byte) (entities.Entities, error) {
    var out entities.Entities
    if err := json.Unmarshal(raw, &out); err != nil {
        return entities.Entities{}, fmt.Errorf("invalid tool input JSON: %w", err)
    }
    return out, nil
}

func (c *ClaudeExtractor) Extract(ctx context.Context, text string) (entities.Entities, error) {
    resp, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
        Model:     c.model,
        MaxTokens: 4096,
        System: []anthropic.TextBlockParam{
            {Text: claudeSystemPrompt},
        },
        Tools: []anthropic.ToolUnionParam{{OfTool: &recordEntitiesTool}},
        Messages: []anthropic.MessageParam{
            anthropic.NewUserMessage(anthropic.NewTextBlock(text)),
        },
    })
    if err != nil {
        return entities.Entities{}, fmt.Errorf("claude request failed: %w", err)
    }

    for _, block := range resp.Content {
        if variant, ok := block.AsAny().(anthropic.ToolUseBlock); ok && variant.Name == "record_entities" {
            return parseToolInput([]byte(variant.JSON.Input.Raw()))
        }
    }
    return entities.Entities{}, fmt.Errorf("claude did not call record_entities")
}
```

**Step 5: Run test to verify it passes**

Run: `cd apps/parser && go test ./internal/extractor/... -v`
Expected: PASS (le test réseau réel n'est pas exécuté ici, seul `parseToolInput` est testé)

**Step 6: Commit**

```bash
git add apps/parser/go.mod apps/parser/go.sum apps/parser/internal/extractor/claude.go apps/parser/internal/extractor/claude_test.go
git commit -m "feat(parser): add ClaudeExtractor fallback via forced tool-use"
```

---

## Task 5 : Orchestrateur avec escalade conditionnelle

**Files:**
- Create: `apps/parser/internal/pipeline/pipeline.go`
- Test: `apps/parser/internal/pipeline/pipeline_test.go`

Logique : tenter `primary` (Ollama) ; si erreur OU résultat vide après validation, tenter `fallback` (Claude). Testable avec des extracteurs fake (pas de vrai réseau).

**Step 1: Write the failing test**

```go
// apps/parser/internal/pipeline/pipeline_test.go
package pipeline

import (
    "context"
    "errors"
    "testing"

    "github.com/S-rvo/BlueWhiteThreat/apps/parser/internal/entities"
)

type fakeExtractor struct {
    result entities.Entities
    err    error
    calls  int
}

func (f *fakeExtractor) Extract(ctx context.Context, text string) (entities.Entities, error) {
    f.calls++
    return f.result, f.err
}

func TestExtractWithFallback_UsesPrimaryWhenSuccessful(t *testing.T) {
    primary := &fakeExtractor{}
    primary.result.CVEs = []string{"CVE-2024-1234"}
    fallback := &fakeExtractor{}

    out, err := ExtractWithFallback(context.Background(), primary, fallback, "text")
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if len(out.CVEs) != 1 {
        t.Fatalf("expected primary result, got %v", out)
    }
    if fallback.calls != 0 {
        t.Fatal("fallback should not be called when primary succeeds")
    }
}

func TestExtractWithFallback_EscalatesOnPrimaryError(t *testing.T) {
    primary := &fakeExtractor{err: errors.New("ollama down")}
    fallback := &fakeExtractor{}
    fallback.result.Malware = []string{"Emotet"}

    out, err := ExtractWithFallback(context.Background(), primary, fallback, "text")
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if len(out.Malware) != 1 || fallback.calls != 1 {
        t.Fatalf("expected fallback used, got out=%v calls=%d", out, fallback.calls)
    }
}

func TestExtractWithFallback_EscalatesOnEmptyResult(t *testing.T) {
    primary := &fakeExtractor{} // résultat vide, pas d'erreur
    fallback := &fakeExtractor{}
    fallback.result.CVEs = []string{"CVE-2024-9999"}

    out, err := ExtractWithFallback(context.Background(), primary, fallback, "text mentioning CVE-2024-9999")
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if len(out.CVEs) != 1 || fallback.calls != 1 {
        t.Fatalf("expected fallback used on empty primary result, got out=%v calls=%d", out, fallback.calls)
    }
}

func TestExtractWithFallback_BothFail(t *testing.T) {
    primary := &fakeExtractor{err: errors.New("ollama down")}
    fallback := &fakeExtractor{err: errors.New("claude down")}

    _, err := ExtractWithFallback(context.Background(), primary, fallback, "text")
    if err == nil {
        t.Fatal("expected error when both extractors fail")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `cd apps/parser && go test ./internal/pipeline/... -v`
Expected: FAIL (build error — package/fonction non définie)

**Step 3: Write minimal implementation**

```go
// apps/parser/internal/pipeline/pipeline.go
package pipeline

import (
    "context"
    "fmt"

    "github.com/S-rvo/BlueWhiteThreat/apps/parser/internal/entities"
    "github.com/S-rvo/BlueWhiteThreat/apps/parser/internal/extractor"
)

// ExtractWithFallback tente primary (typiquement Ollama, gratuit) puis, en cas
// d'échec ou de résultat vide après validation, escalade vers fallback
// (typiquement Claude). Ça borne le coût du fallback aux cas difficiles.
func ExtractWithFallback(ctx context.Context, primary, fallback extractor.Extractor, text string) (entities.Entities, error) {
    result, err := primary.Extract(ctx, text)
    if err == nil {
        cleaned := entities.ValidateAndClean(result)
        if !cleaned.IsEmpty() {
            return cleaned, nil
        }
    }

    fbResult, fbErr := fallback.Extract(ctx, text)
    if fbErr != nil {
        return entities.Entities{}, fmt.Errorf("primary and fallback both failed: primary_err=%v fallback_err=%w", err, fbErr)
    }
    return entities.ValidateAndClean(fbResult), nil
}
```

**Step 4: Run test to verify it passes**

Run: `cd apps/parser && go test ./internal/pipeline/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add apps/parser/internal/pipeline/
git commit -m "feat(parser): add primary/fallback extraction orchestrator"
```

---

## Task 6 : Persistance MongoDB (lecture articles / écriture entities)

**Files:**
- Create: `apps/parser/internal/db/mongo.go`
- Test: nécessite une instance Mongo (test d'intégration, cf. Step 4)

Réutilise le pattern de connexion de `apps/scraper/internal/db/mongo.go` (env vars `MONGO_HOST`, `MONGO_PORT`, `MONGO_INITDB_ROOT_USERNAME`, `MONGO_INITDB_ROOT_PASSWORD`).

**Step 1: Implémentation**

```go
// apps/parser/internal/db/mongo.go
package db

import (
    "context"
    "os"
    "time"

    "go.mongodb.org/mongo-driver/bson"
    "go.mongodb.org/mongo-driver/mongo"
    "go.mongodb.org/mongo-driver/mongo/options"

    "github.com/S-rvo/BlueWhiteThreat/apps/parser/internal/entities"
)

func getEnvOrDefault(key, def string) string {
    if v, ok := os.LookupEnv(key); ok && v != "" {
        return v
    }
    return def
}

func ConnectMongo(ctx context.Context) (*mongo.Client, error) {
    user := getEnvOrDefault("MONGO_INITDB_ROOT_USERNAME", "root")
    pass := getEnvOrDefault("MONGO_INITDB_ROOT_PASSWORD", "root")
    host := getEnvOrDefault("MONGO_HOST", "localhost")
    port := getEnvOrDefault("MONGO_PORT", "27017")
    uri := "mongodb://" + user + ":" + pass + "@" + host + ":" + port + "/"

    ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
    defer cancel()
    return mongo.Connect(ctx, options.Client().ApplyURI(uri))
}

type UnparsedArticle struct {
    ID      any    `bson:"_id"`
    Content string `bson:"content"`
}

// FetchUnparsedArticles renvoie les articles n'ayant pas encore de champ "entities".
func FetchUnparsedArticles(ctx context.Context, client *mongo.Client, limit int64) ([]UnparsedArticle, error) {
    coll := client.Database("BlueWhiteThreat").Collection("article")
    filter := bson.M{"entities": bson.M{"$exists": false}}
    cur, err := coll.Find(ctx, filter, options.Find().SetLimit(limit))
    if err != nil {
        return nil, err
    }
    defer cur.Close(ctx)

    var out []UnparsedArticle
    if err := cur.All(ctx, &out); err != nil {
        return nil, err
    }
    return out, nil
}

// SaveEntities écrit le résultat d'extraction sur le document article correspondant.
// Un résultat vide est aussi écrit (tableaux vides) pour marquer l'article comme traité
// et éviter de le retraiter en boucle.
func SaveEntities(ctx context.Context, client *mongo.Client, articleID any, ents entities.Entities) error {
    coll := client.Database("BlueWhiteThreat").Collection("article")
    _, err := coll.UpdateOne(ctx,
        bson.M{"_id": articleID},
        bson.M{"$set": bson.M{"entities": ents}},
    )
    return err
}
```

**Step 2: Test d'intégration (nécessite Mongo local via docker-compose)**

```go
// apps/parser/internal/db/mongo_integration_test.go
//go:build integration

package db

import (
    "context"
    "testing"

    "github.com/S-rvo/BlueWhiteThreat/apps/parser/internal/entities"
)

func TestSaveAndFetchEntities(t *testing.T) {
    ctx := context.Background()
    client, err := ConnectMongo(ctx)
    if err != nil {
        t.Fatalf("connect: %v", err)
    }
    defer client.Disconnect(ctx)

    coll := client.Database("BlueWhiteThreat").Collection("article")
    res, err := coll.InsertOne(ctx, map[string]any{"content": "test article"})
    if err != nil {
        t.Fatalf("insert: %v", err)
    }

    ents := entities.Entities{}
    ents.CVEs = []string{"CVE-2024-1111"}
    if err := SaveEntities(ctx, client, res.InsertedID, ents); err != nil {
        t.Fatalf("save: %v", err)
    }

    unparsed, err := FetchUnparsedArticles(ctx, client, 10)
    if err != nil {
        t.Fatalf("fetch: %v", err)
    }
    for _, a := range unparsed {
        if a.ID == res.InsertedID {
            t.Fatal("article should no longer be unparsed after SaveEntities")
        }
    }
}
```

Run (avec `docker-compose up -d mongodb` au préalable) : `cd apps/parser && go test -tags=integration ./internal/db/... -v`
Expected: PASS

**Step 3: Commit**

```bash
git add apps/parser/internal/db/mongo.go apps/parser/internal/db/mongo_integration_test.go
git commit -m "feat(parser): add MongoDB read/write for entity extraction"
```

---

## Task 7 : Persistance Neo4j (nœuds entités + relations)

**Files:**
- Create: `apps/parser/internal/db/neo4j.go`

Réutilise le pattern de `apps/crawler/internal/db/neo4j.go` (driver global, `MERGE` pour l'idempotence).

**Step 1: Implémentation**

```go
// apps/parser/internal/db/neo4j.go
package db

import (
    "context"

    "github.com/neo4j/neo4j-go-driver/v5/neo4j"

    "github.com/S-rvo/BlueWhiteThreat/apps/parser/internal/entities"
)

var driver neo4j.DriverWithContext

func InitNeo4jDriver(uri, username, password string) error {
    var err error
    driver, err = neo4j.NewDriverWithContext(uri, neo4j.BasicAuth(username, password, ""))
    return err
}

func CloseNeo4j() {
    if driver != nil {
        driver.Close(context.Background())
    }
}

// entityGroups associe un label Neo4j à la liste de valeurs correspondante.
func entityGroups(e entities.Entities) map[string][]string {
    return map[string][]string{
        "IP":             e.IOCs.IPs,
        "Hash":           e.IOCs.Hashes,
        "Domain":         e.IOCs.Domains,
        "URL":            e.IOCs.URLs,
        "CVE":            e.CVEs,
        "Malware":        e.Malware,
        "ThreatActor":    e.ThreatActors,
        "Infrastructure": e.Infrastructure,
    }
}

// SaveEntityGraph crée un noeud Article (si absent), un noeud par entité typée,
// et une relation MENTIONED_IN entre chaque entité et l'article source.
func SaveEntityGraph(ctx context.Context, articleID string, e entities.Entities) error {
    session := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
    defer session.Close(ctx)

    for label, values := range entityGroups(e) {
        for _, v := range values {
            _, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
                query := `MERGE (a:Article {id: $articleID})
                          MERGE (e:` + label + ` {value: $value})
                          MERGE (e)-[:MENTIONED_IN]->(a)`
                _, err := tx.Run(ctx, query, map[string]any{"articleID": articleID, "value": v})
                return nil, err
            })
            if err != nil {
                return err
            }
        }
    }
    return nil
}
```

> Note : `label` est injecté dans la requête Cypher car Neo4j n'autorise pas les labels paramétrés — `label` provient uniquement des 8 clés fixes de `entityGroups` (jamais d'une valeur utilisateur), donc pas d'injection possible.

**Step 2: Commit**

```bash
git add apps/parser/internal/db/neo4j.go
git commit -m "feat(parser): add Neo4j entity graph persistence"
```

---

## Task 8 : `main.go` — boucle du service

**Files:**
- Modify: `apps/parser/cmd/main.go`

**Step 1: Implémentation**

```go
package main

import (
    "context"
    "log"
    "os"
    "strconv"
    "time"

    "github.com/joho/godotenv"

    "github.com/S-rvo/BlueWhiteThreat/apps/parser/internal/db"
    "github.com/S-rvo/BlueWhiteThreat/apps/parser/internal/extractor"
    "github.com/S-rvo/BlueWhiteThreat/apps/parser/internal/pipeline"
)

var defaultIntervalMin = 10

func main() {
    if err := godotenv.Load(); err != nil {
        log.Println("Aucun .env trouvé ou problème de lecture.")
    }
    if i, err := strconv.Atoi(os.Getenv("PARSER_INTERVAL_MIN")); err == nil && i > 0 {
        defaultIntervalMin = i
    }

    if err := db.InitNeo4jDriver(os.Getenv("NEO4J_URI"), os.Getenv("NEO4J_USER"), os.Getenv("NEO4J_PASSWORD")); err != nil {
        log.Fatalf("Error initializing Neo4j: %v", err)
    }
    defer db.CloseNeo4j()

    ollamaHost := getEnvOrDefault("OLLAMA_HOST", "http://localhost:11434")
    ollamaModel := getEnvOrDefault("OLLAMA_MODEL", "qwen2.5:14b-instruct")
    primary := extractor.NewOllamaExtractor(ollamaHost, ollamaModel)
    fallback := extractor.NewClaudeExtractor()

    log.Printf("Lancement du parser (interval: %d min)", defaultIntervalMin)
    runOnce(primary, fallback)

    ticker := time.NewTicker(time.Duration(defaultIntervalMin) * time.Minute)
    defer ticker.Stop()
    for range ticker.C {
        runOnce(primary, fallback)
    }
}

func getEnvOrDefault(key, def string) string {
    if v, ok := os.LookupEnv(key); ok && v != "" {
        return v
    }
    return def
}

func runOnce(primary, fallback interface {
    Extract(ctx context.Context, text string) (interface{}, error)
}) {
    // Implémentation détaillée : voir Step 2 (signature corrigée ci-dessous).
}
```

> Le pseudo-code `runOnce` ci-dessus a une signature invalide (interface générique mal formée) — corrigé au Step 2 avec les vrais types `extractor.Extractor`.

**Step 2: Version corrigée de `runOnce`**

```go
func runOnce(primary, fallback extractor.Extractor) {
    ctx := context.Background()
    client, err := db.ConnectMongo(ctx)
    if err != nil {
        log.Printf("Erreur connexion MongoDB: %v", err)
        return
    }
    defer client.Disconnect(ctx)

    articles, err := db.FetchUnparsedArticles(ctx, client, 50)
    if err != nil {
        log.Printf("Erreur lecture articles: %v", err)
        return
    }
    log.Printf("Parser: %d articles à traiter", len(articles))

    for _, a := range articles {
        ents, err := pipeline.ExtractWithFallback(ctx, primary, fallback, a.Content)
        if err != nil {
            log.Printf("Erreur extraction article %v: %v", a.ID, err)
            continue
        }
        if err := db.SaveEntities(ctx, client, a.ID, ents); err != nil {
            log.Printf("Erreur écriture Mongo article %v: %v", a.ID, err)
            continue
        }
        if err := db.SaveEntityGraph(ctx, idToString(a.ID), ents); err != nil {
            log.Printf("Erreur écriture Neo4j article %v: %v", a.ID, err)
        }
    }
}

func idToString(id any) string {
    return fmt.Sprintf("%v", id)
}
```

(Remplacer le premier `runOnce` placeholder par cette version ; ajouter `"fmt"` aux imports.)

**Step 3: Vérifier la compilation**

Run: `cd apps/parser && go build ./...`
Expected: aucune erreur

**Step 4: Commit**

```bash
git add apps/parser/cmd/main.go
git commit -m "feat(parser): wire main loop (Mongo -> extract -> Mongo+Neo4j)"
```

---

## Task 9 : Intégration Docker Compose (Ollama + service parser)

**Files:**
- Create: `apps/parser/Dockerfile.dev` (copier le style de `apps/scraper/Dockerfile.dev`)
- Modify: `docker-compose.yaml`

**Step 1: Dockerfile.dev**

```dockerfile
FROM golang:1.24
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
CMD ["go", "run", "./cmd"]
```

**Step 2: Ajouter les services à `docker-compose.yaml`**

```yaml
  ollama:
    image: ollama/ollama:latest
    container_name: ollama_service
    ports:
      - "11434:11434"
    volumes:
      - ollama_data:/root/.ollama

  parser:
    build:
      context: ./apps/parser
      dockerfile: Dockerfile.dev
    container_name: parser_service
    volumes:
      - ./apps/parser:/app
    depends_on:
      - mongodb
      - neo4j
      - ollama
    environment:
      - MONGO_HOST=mongodb
      - MONGO_PORT=27017
      - NEO4J_URI=bolt://neo4j:7687
      - NEO4J_USER=neo4j
      - NEO4J_PASSWORD=password
      - OLLAMA_HOST=http://ollama:11434
      - OLLAMA_MODEL=qwen2.5:14b-instruct
      - PARSER_INTERVAL_MIN=10
    env_file:
      - ./apps/scraper/.env
```

Et ajouter `ollama_data:` à la section `volumes:` en fin de fichier.

**Step 3: Vérifier que le service démarre**

Run: `docker compose up -d ollama mongodb neo4j parser`
Puis, une fois `ollama` up : `docker exec ollama_service ollama pull qwen2.5:14b-instruct`
Run: `docker compose logs -f parser`
Expected: logs `Lancement du parser (interval: 10 min)` puis `Parser: N articles à traiter`

**Step 4: Commit**

```bash
git add apps/parser/Dockerfile.dev docker-compose.yaml
git commit -m "chore(parser): wire Ollama + parser service into docker-compose"
```

---

## Task 10 : Vérification end-to-end manuelle

**Step 1:** Insérer un article de test contenant un IOC connu directement en Mongo (via `mongo-express` sur `:8082` ou `mongosh`), par ex. contenu = `"Le malware Emotet utilise l'IP 45.146.164.110 et exploite CVE-2023-1234."`

**Step 2:** Attendre le prochain tick du parser (ou relancer le conteneur pour déclencher le run immédiat).

**Step 3:** Vérifier dans Mongo que le document a désormais un champ `entities` non vide cohérent avec le texte.

**Step 4:** Vérifier dans Neo4j Browser (`:7474`) avec la requête :
```cypher
MATCH (e)-[:MENTIONED_IN]->(a:Article) RETURN e, a LIMIT 25
```
Expected: des nœuds `Malware`, `IP`, `CVE` reliés au nœud `Article`.

**Step 5:** Si l'extraction Ollama échoue systématiquement (modèle non tiré, service down), vérifier dans les logs que l'escalade vers Claude s'est bien déclenchée (nécessite `ANTHROPIC_API_KEY` renseignée dans `apps/parser/.env`).

---

## Hors périmètre (voir design doc)

Scoring/triage, enrichissement externe (VirusTotal, CVE feeds, MISP), élargissement des sources, affichage front (`ioc.vue`) — priorités suivantes de la roadmap, non traitées par ce plan.
