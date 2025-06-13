package main

import (
    "bytes"
    "fmt"
    "io/ioutil"
    "net/http"

    "github.com/PuerkitoBio/goquery"
    "golang.org/x/net/proxy"
)

func main() {
    // Configuration du proxy socks5 (Tor par défaut sur 127.0.0.1:9050)
    dialer, err := proxy.SOCKS5("tcp", "127.0.0.1:9050", nil, proxy.Direct)
    if err != nil {
        panic(err)
    }

    transport := &http.Transport{
        Dial: dialer.Dial,
    }

    client := &http.Client{
        Transport: transport,
    }

    // Préparation de la requête POST
    postData := []byte("translit=Jordan_Kuwait_Bank")

    req, err := http.NewRequest("POST", "http://ransomocmou6mnbquqz44ewosbkjk3o5qjsl3orawojexfook2j7esad.onion/controllers/news_card", bytes.NewBuffer(postData))
    if err != nil {
        panic(err)
    }

    // Ajout des headers
    req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
    req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64; rv:128.0) Gecko/20100101 Firefox/128.0")
    req.Header.Set("Accept", "*/*")
    req.Header.Set("Referer", "http://ransomocmou6mnbquqz44ewosbkjk3o5qjsl3orawojexfook2j7esad.onion/news")
    req.Header.Set("X-Requested-With", "XMLHttpRequest")
    req.Header.Set("Cookie", "PHPSESSID=c1ld61vtgo7ubcsj24lfq3vk41; token_user=bvHRr10OqsGF4j7Xea8BkoPgtCDMcUJ")

    // Envoi de la requête
    resp, err := client.Do(req)
    if err != nil {
        panic(err)
    }
    defer resp.Body.Close()

    // Lecture du body
    body, err := ioutil.ReadAll(resp.Body)
    if err != nil {
        panic(err)
    }

    // Parsing HTML avec goquery
    doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
    if err != nil {
        panic(err)
    }

    // Parcours des timeline-item
    doc.Find("li.timeline-item").Each(func(i int, s *goquery.Selection) {
        translit, _ := s.Attr("data-translit")
        titre := s.Find("h3").First().Text()
        description := s.Find("p.publication-description").First().Text()
        date := s.Find("div.date-view").First().Text()

        fmt.Println("==============")
        fmt.Printf("data-translit: %s\n", translit)
        fmt.Printf("Titre: %s\n", titre)
        fmt.Printf("Description: %s\n", description)
        fmt.Printf("Date: %s\n", date)
        fmt.Println("==============")
    })
}

