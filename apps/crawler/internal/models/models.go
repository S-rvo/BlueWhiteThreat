package models

import (
    "time"
)

// URLEntry représente une entrée dans la collection des URLs
type URLEntry struct {
    URL     string    `bson:"url"`
    Visited bool      `bson:"visited"`
    AddedAt time.Time `bson:"added_at"`
}

// PageEntry représente une page web scrapée
type PageEntry struct {
    URL       string    `bson:"url"`
    Title     string    `bson:"title"`
    Content   string    `bson:"content"`
    Links     []string  `bson:"links"`
    ScrapedAt time.Time `bson:"scraped_at"`
}

// ErrorEntry représente une erreur de crawling
type ErrorEntry struct {
    URL       string    `bson:"url"`
    Error     string    `bson:"error"`
    Timestamp time.Time `bson:"timestamp"`
}
