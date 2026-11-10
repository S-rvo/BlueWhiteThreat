module github.com/S-rvo/BlueWhiteThreat/apps/crawler

go 1.24

require (
	github.com/S-rvo/BlueWhiteThreat/pkg/csamfilter v0.0.0
	github.com/gocolly/colly v1.2.0
	github.com/neo4j/neo4j-go-driver/v5 v5.28.1
	github.com/redis/go-redis/v9 v9.7.3
	golang.org/x/net v0.38.0
)

replace github.com/S-rvo/BlueWhiteThreat/pkg/csamfilter => ../../pkg/csamfilter

// toolchain go1.23.7

require (
	github.com/PuerkitoBio/goquery v1.10.2 // indirect
	github.com/alicebob/miniredis/v2 v2.35.0 // indirect
	github.com/andybalholm/cascadia v1.3.3 // indirect
	github.com/antchfx/htmlquery v1.3.4 // indirect
	github.com/antchfx/xmlquery v1.4.4 // indirect
	github.com/antchfx/xpath v1.3.3 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/gobwas/glob v0.2.3 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/kennygrant/sanitize v1.2.4 // indirect
	github.com/saintfish/chardet v0.0.0-20230101081208-5e3ef4b5456d // indirect
	github.com/temoto/robotstxt v1.1.2 // indirect
	github.com/yuin/gopher-lua v1.1.1 // indirect
	golang.org/x/text v0.23.0 // indirect
	google.golang.org/appengine v1.6.8 // indirect
	google.golang.org/protobuf v1.33.0 // indirect
)
