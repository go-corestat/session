package session

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

type Store struct {
	config Config
}

type Session struct {
	ID        string    `json:"id"`
	Sub       string    `json:"sub"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Username  string    `json:"username"`
	TenantID  string    `json:"tenant_id"`
	CreatedAt time.Time `json:"created_at"`
}

func New(config Config) *Store {
	return &Store{config: config}
}

func (s *Store) Create(ctx context.Context, session Session) (string, error) {
	if session.Sub == "" {
		return "", errors.New("session subject is required")
	}

	id, err := generateSessionID()
	if err != nil {
		return "", err
	}
	session.ID = id
	session.CreatedAt = time.Now().UTC()

	payload, err := json.Marshal(session)
	if err != nil {
		return "", fmt.Errorf("marshal session: %w", err)
	}

	key := s.redisKey(id)
	ttlSeconds := strconv.Itoa(int(s.config.SessionTTL.Seconds()))
	if ttlSeconds == "0" {
		ttlSeconds = strconv.Itoa(8 * 60 * 60)
	}

	if _, err := s.command(ctx, "SET", key, string(payload), "EX", ttlSeconds); err != nil {
		return "", fmt.Errorf("persist session: %w", err)
	}

	return id, nil
}

func (s *Store) Get(ctx context.Context, sessionID string) (Session, error) {
	key := s.redisKey(sessionID)
	resp, err := s.command(ctx, "GET", key)
	if err != nil {
		return Session{}, err
	}
	if resp == "" {
		return Session{}, errors.New("session not found")
	}

	var session Session
	if err := json.Unmarshal([]byte(resp), &session); err != nil {
		return Session{}, fmt.Errorf("decode session payload: %w", err)
	}
	return session, nil
}

func (s *Store) Delete(ctx context.Context, sessionID string) error {
	key := s.redisKey(sessionID)
	_, err := s.command(ctx, "DEL", key)
	return err
}

func (s *Store) Ping(ctx context.Context) error {
	_, err := s.command(ctx, "PING")
	return err
}

func (s *Store) TTL() time.Duration {
	if s.config.SessionTTL <= 0 {
		return 8 * time.Hour
	}
	return s.config.SessionTTL
}

func (s *Store) CookieName() string {
	return s.config.CookieName
}

func (s *Store) CookieDomain() string {
	return s.config.CookieDomain
}

func (s *Store) CookiePath() string {
	if strings.TrimSpace(s.config.CookiePath) == "" {
		return "/"
	}
	return s.config.CookiePath
}

func (s *Store) CookieSecure() bool {
	return s.config.CookieSecure
}

func (s *Store) CookieHTTPOnly() bool {
	return s.config.CookieHTTPOnly
}

func (s *Store) CookieSameSite() string {
	return s.config.CookieSameSite
}

func (s *Store) redisKey(sessionID string) string {
	return "session:" + sessionID
}

func generateSessionID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate session id: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func (s *Store) command(ctx context.Context, args ...string) (string, error) {
	dialer := net.Dialer{Timeout: 5 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", s.config.RedisAddr)
	if err != nil {
		return "", fmt.Errorf("connect redis: %w", err)
	}
	defer conn.Close()

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	} else {
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	}

	reader := bufio.NewReader(conn)

	if s.config.RedisPassword != "" {
		if err := writeRESP(conn, "AUTH", s.config.RedisPassword); err != nil {
			return "", err
		}
		if _, err := readRESP(reader); err != nil {
			return "", fmt.Errorf("redis auth failed: %w", err)
		}
	}

	if s.config.RedisDB > 0 {
		if err := writeRESP(conn, "SELECT", strconv.Itoa(s.config.RedisDB)); err != nil {
			return "", err
		}
		if _, err := readRESP(reader); err != nil {
			return "", fmt.Errorf("redis select db failed: %w", err)
		}
	}

	if err := writeRESP(conn, args...); err != nil {
		return "", err
	}

	resp, err := readRESP(reader)
	if err != nil {
		return "", err
	}
	return resp, nil
}

func writeRESP(conn net.Conn, args ...string) error {
	var b strings.Builder
	b.WriteString("*")
	b.WriteString(strconv.Itoa(len(args)))
	b.WriteString("\r\n")
	for _, arg := range args {
		b.WriteString("$")
		b.WriteString(strconv.Itoa(len(arg)))
		b.WriteString("\r\n")
		b.WriteString(arg)
		b.WriteString("\r\n")
	}
	_, err := conn.Write([]byte(b.String()))
	if err != nil {
		return fmt.Errorf("write redis command: %w", err)
	}
	return nil
}

func readRESP(reader *bufio.Reader) (string, error) {
	prefix, err := reader.ReadByte()
	if err != nil {
		return "", fmt.Errorf("read redis response prefix: %w", err)
	}

	line, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("read redis response: %w", err)
	}
	line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")

	switch prefix {
	case '+':
		return line, nil
	case '-':
		return "", fmt.Errorf("redis error: %s", line)
	case ':':
		return line, nil
	case '$':
		size, err := strconv.Atoi(line)
		if err != nil {
			return "", fmt.Errorf("parse redis bulk length: %w", err)
		}
		if size == -1 {
			return "", nil
		}

		payload := make([]byte, size+2)
		if _, err := io.ReadFull(reader, payload); err != nil {
			return "", fmt.Errorf("read redis bulk payload: %w", err)
		}
		return string(payload[:size]), nil
	default:
		return "", fmt.Errorf("unsupported redis response type: %q", prefix)
	}
}
