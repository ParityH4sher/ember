package auth

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

// A minimal Redis client: RESP2 over TCP, enough for the session store
// (AUTH, PING, SET with PX, GET, DEL, GETDEL). ADR 005 prefers the standard
// library, and these few commands do not justify a dependency. It works with
// Redis 6.2 or newer and with Valkey.

const (
	redisDialTimeout = 2 * time.Second
	redisIOTimeout   = 2 * time.Second
	redisMaxOpen     = 16
	redisMaxIdle     = 4
	// Sessions hold an ID token, typically a few kilobytes.
	redisMaxBulk = 1 << 20
	redisMaxLine = 4 << 10
)

// errRedisProtocol means the server sent something this client does not
// expect. The connection is dropped.
var errRedisProtocol = errors.New("redis: protocol error")

// redisError is an error reply, such as -WRONGPASS or -ERR unknown command.
// The connection stays usable.
type redisError string

func (e redisError) Error() string { return "redis: " + string(e) }

// redisReply is one RESP2 reply other than an error. Arrays are not needed by
// the commands this client sends and are refused.
type redisReply struct {
	kind byte // '+' simple string, ':' integer, '$' bulk string
	str  string
	num  int64
	null bool // a null bulk string ($-1)
}

type redisClient struct {
	addr     string
	password string // empty: no AUTH
	dial     func(ctx context.Context, network, addr string) (net.Conn, error)

	slots chan struct{} // bounds open connections
	mu    sync.Mutex
	idle  []*redisConn
}

type redisConn struct {
	conn net.Conn
	r    *bufio.Reader
	w    *bufio.Writer
}

func newRedisClient(addr, password string) *redisClient {
	d := &net.Dialer{Timeout: redisDialTimeout, KeepAlive: 30 * time.Second}
	return &redisClient{
		addr:     addr,
		password: password,
		dial:     d.DialContext,
		slots:    make(chan struct{}, redisMaxOpen),
	}
}

// do sends one command and reads its reply. An error reply is returned as a
// redisError. Network and protocol errors close the connection; the next
// command dials again. A command on a reused idle connection that fails
// before any reply byte arrives is retried once on a fresh connection,
// because Redis may have restarted since the connection was last used.
func (c *redisClient) do(ctx context.Context, args ...string) (redisReply, error) {
	select {
	case c.slots <- struct{}{}:
	case <-ctx.Done():
		return redisReply{}, ctx.Err()
	}
	defer func() { <-c.slots }()

	rc, reused := c.takeIdle(), true
	if rc == nil {
		reused = false
	}
	for {
		if rc == nil {
			var err error
			if rc, err = c.connect(ctx); err != nil {
				return redisReply{}, err
			}
		}
		reply, started, err := rc.roundTrip(ctx, args)
		var replyErr redisError
		if err == nil || errors.As(err, &replyErr) {
			c.putIdle(rc)
			return reply, err
		}
		rc.conn.Close()
		if !reused || started || ctx.Err() != nil {
			return redisReply{}, err
		}
		rc, reused = nil, false
	}
}

func (c *redisClient) takeIdle() *redisConn {
	c.mu.Lock()
	defer c.mu.Unlock()
	if n := len(c.idle); n > 0 {
		rc := c.idle[n-1]
		c.idle = c.idle[:n-1]
		return rc
	}
	return nil
}

func (c *redisClient) putIdle(rc *redisConn) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.idle) < redisMaxIdle {
		c.idle = append(c.idle, rc)
		return
	}
	rc.conn.Close()
}

// close drops idle connections. Connections in use close when their command ends.
func (c *redisClient) close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, rc := range c.idle {
		rc.conn.Close()
	}
	c.idle = nil
}

func (c *redisClient) connect(ctx context.Context) (*redisConn, error) {
	conn, err := c.dial(ctx, "tcp", c.addr)
	if err != nil {
		return nil, fmt.Errorf("redis: %w", err)
	}
	rc := &redisConn{conn: conn, r: bufio.NewReader(conn), w: bufio.NewWriter(conn)}
	if c.password != "" {
		if _, _, err := rc.roundTrip(ctx, []string{"AUTH", c.password}); err != nil {
			conn.Close()
			var replyErr redisError
			if errors.As(err, &replyErr) {
				// Do not echo the server's text: it is about the password.
				return nil, errors.New("redis: authentication failed")
			}
			return nil, err
		}
	}
	return rc, nil
}

// roundTrip writes a command and reads its reply. started reports whether any
// part of a reply arrived, after which a retry could repeat the command.
func (rc *redisConn) roundTrip(ctx context.Context, args []string) (reply redisReply, started bool, err error) {
	deadline := time.Now().Add(redisIOTimeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if err := rc.conn.SetDeadline(deadline); err != nil {
		return redisReply{}, false, err
	}
	fmt.Fprintf(rc.w, "*%d\r\n", len(args))
	for _, arg := range args {
		fmt.Fprintf(rc.w, "$%d\r\n%s\r\n", len(arg), arg)
	}
	if err := rc.w.Flush(); err != nil {
		return redisReply{}, false, fmt.Errorf("redis: %w", err)
	}
	if _, err := rc.r.Peek(1); err != nil {
		return redisReply{}, false, fmt.Errorf("redis: %w", err)
	}
	reply, err = readRedisReply(rc.r)
	return reply, true, err
}

func readRedisReply(r *bufio.Reader) (redisReply, error) {
	line, err := readRedisLine(r)
	if err != nil {
		return redisReply{}, err
	}
	if line == "" {
		return redisReply{}, errRedisProtocol
	}
	kind, rest := line[0], line[1:]
	switch kind {
	case '+':
		return redisReply{kind: kind, str: rest}, nil
	case '-':
		return redisReply{}, redisError(rest)
	case ':':
		n, err := strconv.ParseInt(rest, 10, 64)
		if err != nil {
			return redisReply{}, errRedisProtocol
		}
		return redisReply{kind: kind, num: n}, nil
	case '$':
		n, err := strconv.Atoi(rest)
		switch {
		case err != nil || n < -1:
			return redisReply{}, errRedisProtocol
		case n == -1:
			return redisReply{kind: kind, null: true}, nil
		case n > redisMaxBulk:
			return redisReply{}, fmt.Errorf("redis: reply of %d bytes exceeds %d", n, redisMaxBulk)
		}
		buf := make([]byte, n+2)
		if _, err := io.ReadFull(r, buf); err != nil {
			return redisReply{}, fmt.Errorf("redis: %w", err)
		}
		if buf[n] != '\r' || buf[n+1] != '\n' {
			return redisReply{}, errRedisProtocol
		}
		return redisReply{kind: kind, str: string(buf[:n])}, nil
	}
	return redisReply{}, errRedisProtocol
}

// readRedisLine reads one CRLF-terminated line of at most redisMaxLine bytes.
func readRedisLine(r *bufio.Reader) (string, error) {
	var b strings.Builder
	for {
		chunk, err := r.ReadSlice('\n')
		if b.Len()+len(chunk) > redisMaxLine {
			return "", fmt.Errorf("redis: reply line exceeds %d bytes", redisMaxLine)
		}
		b.Write(chunk)
		if err == nil {
			break
		}
		if !errors.Is(err, bufio.ErrBufferFull) {
			return "", fmt.Errorf("redis: %w", err)
		}
	}
	line := b.String()
	if !strings.HasSuffix(line, "\r\n") {
		return "", errRedisProtocol
	}
	return line[:len(line)-2], nil
}

// ping checks that Redis answers and accepts the password.
func (c *redisClient) ping(ctx context.Context) error {
	reply, err := c.do(ctx, "PING")
	if err != nil {
		return err
	}
	if reply.kind != '+' || reply.str != "PONG" {
		return errRedisProtocol
	}
	return nil
}

// set stores value under key for ttl (SET key value PX ms).
func (c *redisClient) set(ctx context.Context, key, value string, ttl time.Duration) error {
	ms := ttl.Milliseconds()
	if ms < 1 {
		ms = 1
	}
	reply, err := c.do(ctx, "SET", key, value, "PX", strconv.FormatInt(ms, 10))
	if err != nil {
		return err
	}
	if reply.kind != '+' || reply.str != "OK" {
		return errRedisProtocol
	}
	return nil
}

// get returns the value of key; ok is false when the key does not exist.
func (c *redisClient) get(ctx context.Context, key string) (value string, ok bool, err error) {
	return c.bulk(ctx, "GET", key)
}

// getDel returns and deletes the value of key atomically (Redis 6.2+).
func (c *redisClient) getDel(ctx context.Context, key string) (value string, ok bool, err error) {
	return c.bulk(ctx, "GETDEL", key)
}

// del deletes key and reports whether it existed.
func (c *redisClient) del(ctx context.Context, key string) (bool, error) {
	reply, err := c.do(ctx, "DEL", key)
	if err != nil {
		return false, err
	}
	if reply.kind != ':' {
		return false, errRedisProtocol
	}
	return reply.num > 0, nil
}

func (c *redisClient) bulk(ctx context.Context, command, key string) (string, bool, error) {
	reply, err := c.do(ctx, command, key)
	if err != nil {
		return "", false, err
	}
	if reply.kind != '$' {
		return "", false, errRedisProtocol
	}
	return reply.str, !reply.null, nil
}
