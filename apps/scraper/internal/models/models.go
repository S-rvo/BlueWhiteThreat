package models

import (
    "time"
)

// CTIData représente les données de Cyber Threat Intelligence
type CTIData struct {
    Title       string    `bson:"title"`
    URL         string    `bson:"url"`
    Content     string    `bson:"content"`
    Category    string    `bson:"category"`
    Source      string    `bson:"source"`
    PublishedAt time.Time `bson:"published_at"`
    ScrapedAt   time.Time `bson:"scraped_at"`
}
