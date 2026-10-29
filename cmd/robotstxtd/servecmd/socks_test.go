package servecmd

import (
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSOCKS5ProxyForwardsHostname(t *testing.T) {
	const password = "s3cret-password"

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/robots.txt" {
			http.NotFound(w, r)
			return
		}
		if strings.Contains(r.Host, password) || strings.Contains(r.URL.String(), password) {
			http.Error(w, "leaked", http.StatusInternalServerError)
			return
		}
		io.WriteString(w, "proxied")
	}))
	t.Cleanup(target.Close)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })

	obsCh := make(chan socksObservation, 4)

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go serveTestSOCKS5(conn, target.Listener.Addr().String(), obsCh)
		}
	}()

	client, err := newHTTPClient("", "socks5://alice:"+password+"@"+ln.Addr().String())
	if err != nil {
		if strings.Contains(err.Error(), password) {
			t.Fatal("client error contains proxy password")
		}
		t.Fatal(err)
	}

	for range 2 {
		res, err := client.Get("http://robots.example.test/robots.txt")
		if err != nil {
			if strings.Contains(err.Error(), password) {
				t.Fatal("request error contains proxy password")
			}
			t.Fatal(err)
		}
		body, err := io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != http.StatusOK || string(body) != "proxied" {
			t.Fatalf("response %d %q", res.StatusCode, body)
		}
	}

	var got []socksObservation
	deadline := time.After(2 * time.Second)
	for len(got) < 2 {
		select {
		case obs := <-obsCh:
			if obs.err != nil {
				if strings.Contains(obs.err.Error(), password) {
					t.Fatal("socks error contains proxy password")
				}
				t.Fatal(obs.err)
			}
			got = append(got, obs)
		case <-deadline:
			t.Fatalf("proxy dials = %d, want 2", len(got))
		}
	}

	for _, obs := range got {
		if obs.host != "robots.example.test" {
			t.Fatalf("forwarded host = %s", obs.host)
		}
		if obs.atyp != 0x03 {
			t.Fatalf("address type = %#x, want domain", obs.atyp)
		}
		if obs.user != "alice" || obs.pass != password {
			t.Fatal("proxy did not receive the configured credentials")
		}
	}
}

func TestSOCKS5ProxyErrorOmitsPassword(t *testing.T) {
	const password = "s3cret-password"

	_, err := newHTTPClient("", "socks5://alice:"+password+"@")
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), password) || strings.Contains(err.Error(), "alice") {
		t.Fatalf("error leaked credentials: %v", err)
	}

	host, port, err := socks5Endpoint("socks5://alice:" + password + "@proxy.example:1080")
	if err != nil {
		t.Fatal(err)
	}
	if host != "proxy.example" || port != "1080" {
		t.Fatalf("endpoint = %s:%s", host, port)
	}
	if strings.Contains(host+port, password) {
		t.Fatal("log endpoint contains password")
	}
}

type socksObservation struct {
	host string
	user string
	pass string
	atyp byte
	err  error
}

func serveTestSOCKS5(conn net.Conn, targetAddr string, obsCh chan<- socksObservation) {
	defer conn.Close()

	host, user, pass, atyp, err := readSOCKS5Connect(conn)
	obsCh <- socksObservation{host: host, user: user, pass: pass, atyp: atyp, err: err}
	if err != nil {
		return
	}

	upstream, err := net.Dial("tcp", targetAddr)
	if err != nil {
		return
	}
	defer upstream.Close()

	if _, err = conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0}); err != nil {
		return
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		io.Copy(upstream, conn)
		upstream.Close()
	}()
	go func() {
		defer wg.Done()
		io.Copy(conn, upstream)
		conn.Close()
	}()
	wg.Wait()
}

func readSOCKS5Connect(conn net.Conn) (host, user, pass string, atyp byte, err error) {
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	defer conn.SetDeadline(time.Time{})

	header := make([]byte, 2)
	if _, err = io.ReadFull(conn, header); err != nil {
		return
	}
	if header[0] != 0x05 {
		err = io.ErrUnexpectedEOF
		return
	}
	methods := make([]byte, int(header[1]))
	if _, err = io.ReadFull(conn, methods); err != nil {
		return
	}

	method := byte(0x00)
	for _, candidate := range methods {
		if candidate == 0x02 {
			method = 0x02
		}
	}
	if _, err = conn.Write([]byte{0x05, method}); err != nil {
		return
	}
	if method == 0x02 {
		verUlen := make([]byte, 2)
		if _, err = io.ReadFull(conn, verUlen); err != nil {
			return
		}
		userBuf := make([]byte, int(verUlen[1]))
		if _, err = io.ReadFull(conn, userBuf); err != nil {
			return
		}
		var plen [1]byte
		if _, err = io.ReadFull(conn, plen[:]); err != nil {
			return
		}
		passBuf := make([]byte, int(plen[0]))
		if _, err = io.ReadFull(conn, passBuf); err != nil {
			return
		}
		user = string(userBuf)
		pass = string(passBuf)
		if _, err = conn.Write([]byte{0x01, 0x00}); err != nil {
			return
		}
	}

	req := make([]byte, 4)
	if _, err = io.ReadFull(conn, req); err != nil {
		return
	}
	if req[1] != 0x01 {
		err = io.ErrUnexpectedEOF
		return
	}
	atyp = req[3]
	switch atyp {
	case 0x03:
		var n [1]byte
		if _, err = io.ReadFull(conn, n[:]); err != nil {
			return
		}
		name := make([]byte, int(n[0]))
		if _, err = io.ReadFull(conn, name); err != nil {
			return
		}
		host = string(name)
	case 0x01:
		buf := make([]byte, 4)
		if _, err = io.ReadFull(conn, buf); err != nil {
			return
		}
		host = net.IP(buf).String()
	case 0x04:
		buf := make([]byte, 16)
		if _, err = io.ReadFull(conn, buf); err != nil {
			return
		}
		host = net.IP(buf).String()
	default:
		err = io.ErrUnexpectedEOF
		return
	}

	var portBuf [2]byte
	if _, err = io.ReadFull(conn, portBuf[:]); err != nil {
		return
	}
	_ = binary.BigEndian.Uint16(portBuf[:])

	return host, user, pass, atyp, nil
}
