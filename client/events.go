package client

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Frame is one /events message, decoded by kind.
type Frame struct {
	Kind string
	Raw  json.RawMessage
}

// ErrResync is returned when the server says the subscription lagged: the
// consumer must re-read state, never treat the gap as data.
var ErrResync = errors.New("events: resync — re-read state")

// Events is the /events firehose client. Rooms: "all" or "market:<mint>".
type Events struct {
	conn net.Conn
	br   *bufio.Reader
	mu   sync.Mutex
}

// DialEvents upgrades to the indexer's /events websocket (wss:// when the
// base is https://).
func DialEvents(ctx context.Context, base string) (*Events, error) {
	u, err := url.Parse(strings.TrimSuffix(base, "/") + "/events")
	if err != nil {
		return nil, err
	}
	switch u.Scheme {
	case "https":
		u.Scheme = "wss"
	case "http":
		u.Scheme = "ws"
	default:
		return nil, fmt.Errorf("events: unsupported scheme %q", u.Scheme)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	keyBytes := make([]byte, 16)
	if _, err := rand.Read(keyBytes); err != nil {
		return nil, err
	}
	key := base64.StdEncoding.EncodeToString(keyBytes)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Sec-WebSocket-Version", "13")
	req.Header.Set("Sec-WebSocket-Key", key)
	dialer := &net.Dialer{Timeout: 15 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", u.Host)
	if err != nil {
		return nil, fmt.Errorf("events: dial: %w", err)
	}
	if err := req.Write(conn); err != nil {
		conn.Close()
		return nil, err
	}
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, req)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("events: handshake: %w", err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		conn.Close()
		return nil, fmt.Errorf("events: handshake: status %d", resp.StatusCode)
	}
	if resp.Header.Get("Sec-WebSocket-Accept") != acceptKey(key) {
		conn.Close()
		return nil, errors.New("events: bad Sec-WebSocket-Accept")
	}
	return &Events{conn: conn, br: br}, nil
}

func acceptKey(key string) string {
	h := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	return base64.StdEncoding.EncodeToString(h[:])
}

// Subscribe sends the room subscription. ≤ 8 rooms per connection (the
// server cap).
func (e *Events) Subscribe(rooms []string) error {
	if len(rooms) > 8 {
		return fmt.Errorf("events: %d rooms, cap 8", len(rooms))
	}
	var payload string
	if len(rooms) == 0 || (len(rooms) == 1 && rooms[0] == "all") {
		payload = `{"subscribe":"all"}`
	} else {
		b, err := json.Marshal(map[string]any{"subscribe": map[string]string{"market": rooms[0]}})
		if err != nil {
			return err
		}
		payload = string(b)
	}
	return e.writeText(payload)
}

// Next returns the next decoded frame. A resync frame surfaces as
// ErrResync.
func (e *Events) Next(ctx context.Context) (Frame, error) {
	if err := e.conn.SetReadDeadline(time.Now().Add(30 * time.Second)); err != nil {
		return Frame{}, err
	}
	op, payload, err := e.readFrame()
	if err != nil {
		return Frame{}, err
	}
	if op == 0x8 { // close
		return Frame{}, io.EOF
	}
	if op != 0x1 { // text only
		return Frame{}, fmt.Errorf("events: unexpected opcode 0x%x", op)
	}
	var f Frame
	if err := json.Unmarshal(payload, &f); err != nil {
		return Frame{}, fmt.Errorf("events: bad frame: %w", err)
	}
	if f.Kind == "resync" {
		return f, ErrResync
	}
	if f.Kind == "" {
		return Frame{}, errors.New("events: frame without kind")
	}
	return f, nil
}

// Close closes the websocket.
func (e *Events) Close() error { return e.conn.Close() }

func (e *Events) writeText(s string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	buf := make([]byte, 0, len(s)+8)
	buf = append(buf, 0x81)
	n := len(s)
	switch {
	case n < 126:
		buf = append(buf, byte(n))
	case n < 65536:
		buf = append(buf, 126, byte(n>>8), byte(n))
	default:
		buf = append(buf, 127)
		var l [8]byte
		binary.BigEndian.PutUint64(l[:], uint64(n))
		buf = append(buf, l[:]...)
	}
	buf = append(buf, s...)
	_, err := e.conn.Write(buf)
	return err
}

func (e *Events) readFrame() (byte, []byte, error) {
	var hdr [2]byte
	if _, err := io.ReadFull(e.br, hdr[:]); err != nil {
		return 0, nil, err
	}
	op := hdr[0] & 0x0f
	masked := hdr[1]&0x80 != 0
	length := uint64(hdr[1] & 0x7f)
	switch length {
	case 126:
		var ext [2]byte
		if _, err := io.ReadFull(e.br, ext[:]); err != nil {
			return 0, nil, err
		}
		length = uint64(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err := io.ReadFull(e.br, ext[:]); err != nil {
			return 0, nil, err
		}
		length = binary.BigEndian.Uint64(ext[:])
	}
	if length > 1<<20 {
		return 0, nil, errors.New("events: frame too large")
	}
	payload := make([]byte, length)
	if masked {
		var mask [4]byte
		if _, err := io.ReadFull(e.br, mask[:]); err != nil {
			return 0, nil, err
		}
		if _, err := io.ReadFull(e.br, payload); err != nil {
			return 0, nil, err
		}
		for i := range payload {
			payload[i] ^= mask[i%4]
		}
	} else if _, err := io.ReadFull(e.br, payload); err != nil {
		return 0, nil, err
	}
	return op, payload, nil
}

// DecodeFrame unmarshals a frame's payload into the row type by kind.
func DecodeFrame(f Frame, out any) error {
	if f.Raw == nil {
		return errors.New("events: empty frame payload")
	}
	return json.Unmarshal(f.Raw, out)
}
