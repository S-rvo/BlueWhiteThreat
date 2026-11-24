package telegram

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestTorClientRejectsUnsafeConfiguration(t *testing.T) {
	for _, address := range []string{"", "http://127.0.0.1:9051", "socks5://127.0.0.1:9051", "socks5h://localhost", "socks5h://localhost:0", "socks5h://localhost:65536", "socks5h://user:pass@localhost:9051", "socks5h://localhost:9051/path", "socks5h://localhost:9051?direct=true"} {
		if _, err := NewTorClient(address); err == nil {
			t.Errorf("accepted %q", address)
		}
	}
}

// The fake proxy accepts an unresolvable .invalid hostname and responds itself.
// Success proves the client passed the name to SOCKS instead of resolving it.
func TestTorClientRemoteDNS(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	result := make(chan error, 1)
	go func() { result <- serveSOCKSFixture(listener) }()
	// Neither environment proxy configuration nor NO_PROXY may bypass Tor.
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	t.Setenv("NO_PROXY", "*")
	client, err := NewTorClient("socks5h://" + listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	body, err := Fetch(ctx, client, "http://no-local-resolution.invalid/preview")
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "through-tor" {
		t.Fatalf("body %q", body)
	}
	if err = <-result; err != nil {
		t.Fatal(err)
	}
}

func serveSOCKSFixture(listener net.Listener) error {
	conn, err := listener.Accept()
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	greeting := make([]byte, 2)
	if _, err = io.ReadFull(conn, greeting); err != nil {
		return err
	}
	if greeting[0] != 5 {
		return fmt.Errorf("wrong SOCKS version")
	}
	methods := make([]byte, int(greeting[1]))
	if _, err = io.ReadFull(conn, methods); err != nil {
		return err
	}
	if _, err = conn.Write([]byte{5, 0}); err != nil {
		return err
	}
	header := make([]byte, 5)
	if _, err = io.ReadFull(conn, header); err != nil {
		return err
	}
	if header[0] != 5 || header[1] != 1 || header[3] != 3 {
		return fmt.Errorf("destination was not sent as SOCKS domain: %v", header)
	}
	name := make([]byte, int(header[4]))
	if _, err = io.ReadFull(conn, name); err != nil {
		return err
	}
	port := make([]byte, 2)
	if _, err = io.ReadFull(conn, port); err != nil {
		return err
	}
	if string(name) != "no-local-resolution.invalid" || binary.BigEndian.Uint16(port) != 80 {
		return fmt.Errorf("unexpected destination %s:%d", name, binary.BigEndian.Uint16(port))
	}
	if _, err = conn.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 80}); err != nil {
		return err
	}
	request, err := http.ReadRequest(bufio.NewReader(conn))
	if err != nil {
		return err
	}
	request.Body.Close()
	if request.URL.Path != "/preview" {
		return fmt.Errorf("unexpected request path")
	}
	_, err = io.WriteString(conn, "HTTP/1.1 200 OK\r\nContent-Length: 11\r\nConnection: close\r\n\r\nthrough-tor")
	return err
}

func TestTorUnavailableNeverFallsBack(t *testing.T) {
	hits := make(chan struct{}, 1)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits <- struct{}{}; fmt.Fprint(w, "direct") }))
	defer target.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	listener.Close()
	client, err := NewTorClient("socks5h://" + address)
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err = Fetch(ctx, client, target.URL)
	if err == nil || !strings.Contains(err.Error(), address) {
		t.Fatalf("expected Tor connection failure: %v", err)
	}
	select {
	case <-hits:
		t.Fatal("direct target received a request")
	default:
	}
}
