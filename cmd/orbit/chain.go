package main

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// chainTunnel is the fire's network channel: the jailed worker has no
// network (the sandbox is netless by design), so runJobFire listens on a
// unix socket in the orbit home, the worker's HTTP transport dials it, and
// this proxy forwards the TLS bytes to the host named in the ClientHello.
func startChainTunnel(sock, targetPort string, hosts []string) (*chainTunnel, error) {
	if err := os.MkdirAll(filepath.Dir(sock), 0o755); err != nil {
		return nil, err
	}
	_ = os.Remove(sock)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(sock, 0o600); err != nil {
		ln.Close()
		return nil, err
	}
	allowed := make(map[string]bool, len(hosts))
	for _, h := range hosts {
		if h = strings.ToLower(strings.TrimSpace(h)); h != "" {
			allowed[h] = true
		}
	}
	t := &chainTunnel{ln: ln, sock: sock, targetPort: targetPort, allowed: allowed}
	t.wg.Add(1)
	go t.serve()
	return t, nil
}

type chainTunnel struct {
	ln         net.Listener
	sock       string
	targetPort string
	// allowed is the server-name allowlist; an empty list forwards nothing.
	allowed map[string]bool
	wg      sync.WaitGroup
}

func (t *chainTunnel) Close() {
	t.ln.Close()
	t.wg.Wait()
	_ = os.Remove(t.sock)
}

func (t *chainTunnel) serve() {
	defer t.wg.Done()
	for {
		c, err := t.ln.Accept()
		if err != nil {
			return
		}
		t.wg.Add(1)
		go func() {
			defer t.wg.Done()
			t.handle(c)
		}()
	}
}

func (t *chainTunnel) handle(c net.Conn) {
	defer c.Close()
	if err := c.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return
	}
	peeked, host, err := readClientHello(c)
	_ = c.SetDeadline(time.Time{})
	if err != nil {
		return
	}
	if !t.allowed[strings.ToLower(host)] {
		fmt.Fprintf(os.Stderr, "orbit: chain tunnel: refused %q (not the indexer, the RPC, or the airdrop host)\n", host)
		return
	}
	rc, err := net.Dial("tcp", net.JoinHostPort(host, t.targetPort))
	if err != nil {
		return
	}
	defer rc.Close()
	go func() {
		_, _ = io.Copy(c, rc)
		_ = c.Close()
	}()
	src := io.MultiReader(bytes.NewReader(peeked), c)
	_, _ = io.Copy(rc, src)
}

func readClientHello(c net.Conn) ([]byte, string, error) {
	hdr := make([]byte, 5)
	if _, err := io.ReadFull(c, hdr); err != nil {
		return nil, "", err
	}
	if hdr[0] != 22 { // TLS record type: handshake
		return nil, "", fmt.Errorf("chain tunnel: not a TLS handshake record (type %#x)", hdr[0])
	}
	n := int(hdr[3])<<8 | int(hdr[4])
	if n == 0 || n > 1<<18 {
		return nil, "", fmt.Errorf("chain tunnel: TLS record too large: %d", n)
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(c, body); err != nil {
		return nil, "", err
	}
	all := append(hdr, body...)
	host, err := clientHelloHost(body)
	if err != nil {
		return nil, "", err
	}
	return all, host, nil
}

func clientHelloHost(body []byte) (string, error) {
	if len(body) < 4 || body[0] != 0x01 {
		return "", fmt.Errorf("chain tunnel: not a ClientHello")
	}
	off := 4
	advance := func(k int) error {
		if off+k > len(body) {
			return fmt.Errorf("chain tunnel: truncated ClientHello")
		}
		off += k
		return nil
	}
	if err := advance(2 + 32); err != nil { // version + random
		return "", err
	}
	sidLen := int(body[off])
	if err := advance(1 + sidLen); err != nil {
		return "", err
	}
	if off+2 > len(body) {
		return "", fmt.Errorf("chain tunnel: truncated ClientHello")
	}
	csLen := int(body[off])<<8 | int(body[off+1])
	if err := advance(2 + csLen); err != nil {
		return "", err
	}
	compLen := int(body[off])
	if err := advance(1 + compLen); err != nil {
		return "", err
	}
	if off+2 > len(body) {
		return "", fmt.Errorf("chain tunnel: truncated ClientHello")
	}
	extLen := int(body[off])<<8 | int(body[off+1])
	off += 2
	end := off + extLen
	if end > len(body) {
		return "", fmt.Errorf("chain tunnel: truncated extensions")
	}
	for off+4 <= end {
		extType := int(body[off])<<8 | int(body[off+1])
		dataLen := int(body[off+2])<<8 | int(body[off+3])
		off += 4
		if off+dataLen > end {
			return "", fmt.Errorf("chain tunnel: truncated extension")
		}
		if extType == 0 { // server_name
			return serverName(body[off : off+dataLen])
		}
		off += dataLen
	}
	return "", fmt.Errorf("chain tunnel: no server name")
}

func serverName(data []byte) (string, error) {
	if len(data) < 5 {
		return "", fmt.Errorf("chain tunnel: truncated server name")
	}
	listLen := int(data[0])<<8 | int(data[1])
	data = data[2:]
	if listLen > len(data) {
		return "", fmt.Errorf("chain tunnel: truncated server name list")
	}
	if len(data) < 3 || data[0] != 0x00 {
		return "", fmt.Errorf("chain tunnel: no host name")
	}
	nameLen := int(data[1])<<8 | int(data[2])
	name := data[3:]
	if nameLen > len(name) {
		return "", fmt.Errorf("chain tunnel: truncated host name")
	}
	return string(name[:nameLen]), nil
}

func fireChainSock(home string) string {
	return filepath.Join(home, ".rig-job-chain.sock")
}
