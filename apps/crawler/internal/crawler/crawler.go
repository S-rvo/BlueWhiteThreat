package crawler

import (
	"fmt"
	"io"
	"net/http"
	"os"

	"golang.org/x/net/proxy"
)

func TorClient() (string, error) {
	proxyAddr := os.Getenv("TOR_PROXY")
	if proxyAddr == "" {
		proxyAddr = "socks5://tor:9050"
	}

	dialer, err := proxy.SOCKS5("tcp", "tor:9050", nil, proxy.Direct)
	if err != nil {
		panic(err)
	}

	client := &http.Client{
		Transport: &http.Transport{
			Dial: dialer.Dial,
		},
	}

	resp, err := client.Get("http://check.torproject.org")
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	fmt.Println(string(body))

	return string(body), nil
}
