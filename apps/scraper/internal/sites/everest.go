package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type APIScrapingResult struct {
    Translit    string                 `json:"translit" bson:"translit"`
    Title       string                 `json:"title" bson:"title"`
    Description string                 `json:"description" bson:"description"`
    Date        string                 `json:"date" bson:"date"`
    Timestamp   time.Time              `json:"timestamp" bson:"timestamp"`
    Source      string                 `json:"source" bson:"source"`
    RawData     map[string]interface{} `json:"raw_data" bson:"raw_data"`
}

type TorAPIClient struct {
    httpClient   *http.Client
    proxyList    []string
    currentProxy int
    userAgent    string
    cookies      map[string]string
    headers      map[string]string
}

// Créer un nouveau client Tor pour API GET
func NewTorAPIClient() (*TorAPIClient, error) {
    // Récupérer les proxies depuis les variables d'environnement
    torProxies := os.Getenv("TOR_PROXY")
    if torProxies == "" {
        torProxies = "tor1:9050,tor2:9050,tor3:9050"
    }

    proxyList := strings.Split(torProxies, ",")
    
    client := &TorAPIClient{
        proxyList: proxyList,
        userAgent: "Mozilla/5.0 (X11; Linux x86_64; rv:128.0) Gecko/20100101 Firefox/128.0",
        cookies: map[string]string{
            "PHPSESSID":  "c1ld61vtgo7ubcsj24lfq3vk41",
            "token_user": "bvHRr10OqsGF4j7Xea8BkoPgtCDMcUJ",
        },
        headers: map[string]string{
            "Accept":             "application/json, text/html, */*",
            "Accept-Language":    "en-US,en;q=0.9",
            "Accept-Encoding":    "gzip, deflate",
            "Cache-Control":      "no-cache",
            "DNT":                "1",
            "Connection":         "keep-alive",
            "Upgrade-Insecure-Requests": "1",
        },
    }

    return client, client.initHTTPClient()
}

// Initialiser le client HTTP avec le proxy Tor
func (t *TorAPIClient) initHTTPClient() error {
    proxyURL, err := url.Parse(fmt.Sprintf("socks5://%s", t.proxyList[t.currentProxy]))
    if err != nil {
        return fmt.Errorf("erreur parsing proxy: %v", err)
    }

    transport := &http.Transport{
        Proxy:                 http.ProxyURL(proxyURL),
        MaxIdleConns:          10,
        IdleConnTimeout:       90 * time.Second,
        TLSHandshakeTimeout:   10 * time.Second,
        ResponseHeaderTimeout: 60 * time.Second,
        DisableCompression:    false,
        DisableKeepAlives:     false,
    }

    t.httpClient = &http.Client{
        Transport: transport,
        Timeout:   120 * time.Second,
    }

    log.Printf("🔗 Client configuré avec proxy: %s", proxyURL.String())
    return nil
}

// Rotation des proxies en cas d'erreur
func (t *TorAPIClient) rotateProxy() error {
    t.currentProxy = (t.currentProxy + 1) % len(t.proxyList)
    log.Printf("🔄 Rotation vers proxy: %s", t.proxyList[t.currentProxy])
    return t.initHTTPClient()
}

// Effectuer une requête GET API avec paramètres
func (t *TorAPIClient) GetAPI(targetURL string, params map[string]string) (*APIScrapingResult, error) {
    // Construire l'URL avec les paramètres GET
    finalURL := t.buildURLWithParams(targetURL, params)
    
    maxRetries := 3
    for attempt := 1; attempt <= maxRetries; attempt++ {
        log.Printf("🎯 GET Tentative %d/%d pour %s", attempt, maxRetries, finalURL)

        result, err := t.makeGETRequest(finalURL)
        if err == nil {
            return result, nil
        }

        log.Printf("❌ Échec tentative %d: %v", attempt, err)

        // Rotation du proxy pour la prochaine tentative
        if attempt < maxRetries {
            if rotateErr := t.rotateProxy(); rotateErr != nil {
                log.Printf("⚠️ Erreur rotation proxy: %v", rotateErr)
            }
            
            waitTime := time.Duration(attempt*15) * time.Second
            log.Printf("⏳ Attente %v avant retry...", waitTime)
            time.Sleep(waitTime)
        }
    }

    return nil, fmt.Errorf("échec après %d tentatives", maxRetries)
}

// Construire URL avec paramètres GET
func (t *TorAPIClient) buildURLWithParams(baseURL string, params map[string]string) string {
    if len(params) == 0 {
        return baseURL
    }

    u, err := url.Parse(baseURL)
    if err != nil {
        log.Printf("⚠️ Erreur parsing URL: %v", err)
        return baseURL
    }

    q := u.Query()
    for key, value := range params {
        q.Set(key, value)
    }
    u.RawQuery = q.Encode()

    return u.String()
}

// Effectuer la requête HTTP GET
func (t *TorAPIClient) makeGETRequest(targetURL string) (*APIScrapingResult, error) {
    ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
    defer cancel()

    req, err := http.NewRequestWithContext(ctx, "GET", targetURL, nil)
    if err != nil {
        return nil, fmt.Errorf("erreur création requête: %v", err)
    }

    // Ajouter les headers
    req.Header.Set("User-Agent", t.userAgent)
    for key, value := range t.headers {
        req.Header.Set(key, value)
    }

    // Ajouter les cookies
    cookieStr := ""
    for name, value := range t.cookies {
        if cookieStr != "" {
            cookieStr += "; "
        }
        cookieStr += fmt.Sprintf("%s=%s", name, value)
    }
    if cookieStr != "" {
        req.Header.Set("Cookie", cookieStr)
    }

    // Ajouter le referer
    if strings.Contains(targetURL, "/controllers/") {
        req.Header.Set("Referer", strings.Replace(targetURL, "/controllers/news_card", "/news", 1))
    }

    log.Printf("📡 GET Request vers: %s", req.URL.String())

    resp, err := t.httpClient.Do(req)
    if err != nil {
        return nil, fmt.Errorf("erreur requête HTTP GET: %v", err)
    }
    defer resp.Body.Close()

    log.Printf("📥 Réponse GET reçue: Status %d", resp.StatusCode)

    if resp.StatusCode != 200 {
        return nil, fmt.Errorf("status code: %d", resp.StatusCode)
    }

    body, err := io.ReadAll(resp.Body)
    if err != nil {
        return nil, fmt.Errorf("erreur lecture body: %v", err)
    }

    log.Printf("📄 Taille réponse: %d bytes", len(body))

    return t.parseResponse(body)
}

// Parser la réponse API
func (t *TorAPIClient) parseResponse(body []byte) (*APIScrapingResult, error) {
    // Afficher un aperçu de la réponse pour debug
    preview := string(body)
    if len(preview) > 200 {
        preview = preview[:200] + "..."
    }
    log.Printf("🔍 Aperçu réponse: %s", preview)

    // Tenter de parser en JSON d'abord
    var jsonResponse map[string]interface{}
    if err := json.Unmarshal(body, &jsonResponse); err == nil {
        log.Println("📋 Réponse détectée comme JSON")
        return t.parseJSONResponse(jsonResponse)
    }

    // Sinon traiter comme HTML/texte
    log.Println("📄 Réponse détectée comme HTML/Text")
    return t.parseHTMLResponse(string(body))
}

// Parser réponse JSON
func (t *TorAPIClient) parseJSONResponse(data map[string]interface{}) (*APIScrapingResult, error) {
    result := &APIScrapingResult{
        Timestamp: time.Now(),
        Source:    "API_JSON_GET",
        RawData:   data,
    }

    // Extraire les champs connus selon la structure de votre API
    if title, ok := data["title"].(string); ok {
        result.Title = title
    }
    if desc, ok := data["description"].(string); ok {
        result.Description = desc
    }
    if date, ok := data["date"].(string); ok {
        result.Date = date
    }
    if translit, ok := data["translit"].(string); ok {
        result.Translit = translit
    }

    // Si les données sont dans un sous-objet
    if items, ok := data["items"].([]interface{}); ok {
        log.Printf("📦 Trouvé %d items dans la réponse", len(items))
        // Traiter le premier item pour l'exemple
        if len(items) > 0 {
            if firstItem, ok := items[0].(map[string]interface{}); ok {
                if title, exists := firstItem["title"].(string); exists {
                    result.Title = title
                }
                if desc, exists := firstItem["description"].(string); exists {
                    result.Description = desc
                }
            }
        }
    }

    log.Printf("✅ Données JSON GET parsées: %s", result.Title)
    return result, nil
}

// Parser réponse HTML (fallback)
func (t *TorAPIClient) parseHTMLResponse(html string) (*APIScrapingResult, error) {
    result := &APIScrapingResult{
        Timestamp: time.Now(),
        Source:    "API_HTML_GET",
        RawData: map[string]interface{}{
            "html_content": html,
            "length":       len(html),
        },
    }

    // Extraire le titre depuis HTML si possible (basique)
    if strings.Contains(html, "<title>") {
        start := strings.Index(html, "<title>") + 7
        end := strings.Index(html[start:], "</title>")
        if end > 0 {
            result.Title = strings.TrimSpace(html[start : start+end])
        }
    }

    // Limiter la description aux premiers caractères
    result.Description = html[:min(500, len(html))]

    log.Printf("✅ Données HTML GET parsées: %d caractères", len(html))
    return result, nil
}

// Fonction principale pour scraper l'API Everest avec GET
func ScrapEverestAPIGet() error {
    log.Println("🎯 Début du scraping API GET Everest...")

    client, err := NewTorAPIClient()
    if err != nil {
        return fmt.Errorf("erreur création client: %v", err)
    }

    // Exemples d'URLs et paramètres GET
    scenarios := []struct {
        Name   string
        URL    string
        Params map[string]string
    }{
        {
            Name: "News Card API",
            URL:  "http://ransomocmou6mnbquqz44ewosbkjk3o5qjsl3orawojexfook2j7esad.onion/controllers/news_card",
            Params: map[string]string{
                "translit": "Jordan_Kuwait_Bank",
            },
        },
        {
            Name: "API Index",
            URL:  "http://ransomocmou6mnbquqz44ewosbkjk3o5qjsl3orawojexfook2j7esad.onion/api/news",
            Params: map[string]string{
                "format": "json",
                "limit":  "10",
            },
        },
        // Ajouter d'autres endpoints selon vos besoins
    }

    var allResults []*APIScrapingResult

    for _, scenario := range scenarios {
        log.Printf("🔍 Scraping: %s", scenario.Name)
        
        result, err := client.GetAPI(scenario.URL, scenario.Params)
        if err != nil {
            log.Printf("❌ Erreur pour %s: %v", scenario.Name, err)
            continue
        }

        allResults = append(allResults, result)

        // Afficher les résultats
        log.Println("==============")
        log.Printf("📍 Source: %s", scenario.Name)
        log.Printf("🏷️  Translit: %s", result.Translit)
        log.Printf("📝 Titre: %s", result.Title)
        log.Printf("📄 Description: %s", truncateString(result.Description, 100))
        log.Printf("📅 Date: %s", result.Date)
        log.Printf("⏰ Timestamp: %s", result.Timestamp.Format("15:04:05"))
        log.Println("==============")

        // Attendre entre les requêtes pour ne pas surcharger
        time.Sleep(2 * time.Second)
    }

    log.Printf("✅ Scraping GET terminé: %d résultats", len(allResults))
    
    // Sauvegarder en base de données ici
    return saveAPIResults(allResults)
}

// Sauvegarder les résultats (à implémenter selon votre DB)
func saveAPIResults(results []*APIScrapingResult) error {
    for _, result := range results {
        log.Printf("💾 Sauvegarde: %s - %s", result.Source, result.Title)
        // TODO: Implémenter votre logique de sauvegarde MongoDB
    }
    return nil
}

// Utilitaires
func truncateString(s string, length int) string {
    if len(s) <= length {
        return s
    }
    return s[:length] + "..."
}

func min(a, b int) int {
    if a < b {
        return a
    }
    return b
}
