package websocket

import (
	"bufio"
	"bytes"
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

const magicGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

type Conn struct {
	conn         net.Conn
	reader       *bufio.Reader
	writeMu      sync.Mutex
	maskOutbound bool
}

func Upgrade(w http.ResponseWriter, r *http.Request) (*Conn, error) {
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		return nil, errors.New("missing websocket upgrade")
	}
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		return nil, errors.New("missing websocket key")
	}
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		return nil, errors.New("response writer does not support hijack")
	}
	netConn, rw, err := hijacker.Hijack()
	if err != nil {
		return nil, err
	}
	accept := acceptKey(key)
	response := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + accept + "\r\n\r\n"
	if _, err := rw.WriteString(response); err != nil {
		_ = netConn.Close()
		return nil, err
	}
	if err := rw.Flush(); err != nil {
		_ = netConn.Close()
		return nil, err
	}
	return &Conn{conn: netConn, reader: rw.Reader, maskOutbound: false}, nil
}

func Dial(rawURL string) (*Conn, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "ws" {
		return nil, fmt.Errorf("only ws:// URLs are supported, got %s", u.Scheme)
	}
	host := u.Host
	if !strings.Contains(host, ":") {
		host += ":80"
	}
	netConn, err := net.DialTimeout("tcp", host, 8*time.Second)
	if err != nil {
		return nil, err
	}
	key := randomKey()
	path := u.RequestURI()
	if path == "" {
		path = "/"
	}
	request := "GET " + path + " HTTP/1.1\r\n" +
		"Host: " + u.Host + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: " + key + "\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n"
	if _, err := netConn.Write([]byte(request)); err != nil {
		_ = netConn.Close()
		return nil, err
	}
	reader := bufio.NewReader(netConn)
	status, err := reader.ReadString('\n')
	if err != nil {
		_ = netConn.Close()
		return nil, err
	}
	if !strings.Contains(status, "101") {
		_ = netConn.Close()
		return nil, fmt.Errorf("websocket upgrade failed: %s", strings.TrimSpace(status))
	}
	var accept string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			_ = netConn.Close()
			return nil, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 && strings.EqualFold(strings.TrimSpace(parts[0]), "Sec-WebSocket-Accept") {
			accept = strings.TrimSpace(parts[1])
		}
	}
	if accept != acceptKey(key) {
		_ = netConn.Close()
		return nil, errors.New("websocket accept key mismatch")
	}
	return &Conn{conn: netConn, reader: reader, maskOutbound: true}, nil
}

func (c *Conn) ReadJSON(v any) error {
	text, err := c.ReadText()
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(text), v)
}

func (c *Conn) WriteJSON(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return c.WriteText(string(b))
}

func (c *Conn) ReadText() (string, error) {
	for {
		opcode, payload, err := c.readFrame()
		if err != nil {
			return "", err
		}
		switch opcode {
		case 0x1:
			return string(payload), nil
		case 0x8:
			return "", io.EOF
		case 0x9:
			_ = c.writeFrame(0xA, payload)
		case 0xA:
			continue
		default:
			continue
		}
	}
}

func (c *Conn) WriteText(text string) error {
	return c.writeFrame(0x1, []byte(text))
}

func (c *Conn) Close() error {
	_ = c.writeFrame(0x8, nil)
	return c.conn.Close()
}

func (c *Conn) readFrame() (byte, []byte, error) {
	header := make([]byte, 2)
	if _, err := io.ReadFull(c.reader, header); err != nil {
		return 0, nil, err
	}
	opcode := header[0] & 0x0f
	masked := header[1]&0x80 != 0
	length := uint64(header[1] & 0x7f)
	switch length {
	case 126:
		var b [2]byte
		if _, err := io.ReadFull(c.reader, b[:]); err != nil {
			return 0, nil, err
		}
		length = uint64(binary.BigEndian.Uint16(b[:]))
	case 127:
		var b [8]byte
		if _, err := io.ReadFull(c.reader, b[:]); err != nil {
			return 0, nil, err
		}
		length = binary.BigEndian.Uint64(b[:])
	}
	var mask [4]byte
	if masked {
		if _, err := io.ReadFull(c.reader, mask[:]); err != nil {
			return 0, nil, err
		}
	}
	if length > 8*1024*1024 {
		return 0, nil, errors.New("websocket frame too large")
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(c.reader, payload); err != nil {
		return 0, nil, err
	}
	if masked {
		for i := range payload {
			payload[i] ^= mask[i%4]
		}
	}
	return opcode, payload, nil
}

func (c *Conn) writeFrame(opcode byte, payload []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	var buf bytes.Buffer
	buf.WriteByte(0x80 | opcode)
	maskBit := byte(0)
	if c.maskOutbound {
		maskBit = 0x80
	}
	length := len(payload)
	switch {
	case length < 126:
		buf.WriteByte(maskBit | byte(length))
	case length <= 65535:
		buf.WriteByte(maskBit | 126)
		var b [2]byte
		binary.BigEndian.PutUint16(b[:], uint16(length))
		buf.Write(b[:])
	default:
		buf.WriteByte(maskBit | 127)
		var b [8]byte
		binary.BigEndian.PutUint64(b[:], uint64(length))
		buf.Write(b[:])
	}
	if c.maskOutbound {
		var mask [4]byte
		if _, err := rand.Read(mask[:]); err != nil {
			return err
		}
		buf.Write(mask[:])
		masked := make([]byte, len(payload))
		copy(masked, payload)
		for i := range masked {
			masked[i] ^= mask[i%4]
		}
		payload = masked
	}
	buf.Write(payload)
	_, err := c.conn.Write(buf.Bytes())
	return err
}

func acceptKey(key string) string {
	sum := sha1.Sum([]byte(key + magicGUID))
	return base64.StdEncoding.EncodeToString(sum[:])
}

func randomKey() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return base64.StdEncoding.EncodeToString(b[:])
}
