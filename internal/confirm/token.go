package confirm

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"sync"
	"time"
)

type Request struct {
	ToolName string
	Database string
	Mode     string
	SQLHash  string
}

type IssuedToken struct {
	Token     string
	ExpiresAt time.Time
}

type Manager struct {
	ttl    time.Duration
	mu     sync.Mutex
	tokens map[string]tokenRecord
}

type tokenRecord struct {
	request   Request
	expiresAt time.Time
}

func NewManager(ttl time.Duration) *Manager {
	return &Manager{
		ttl:    ttl,
		tokens: make(map[string]tokenRecord),
	}
}

func (m *Manager) TTL() time.Duration {
	return m.ttl
}

func (m *Manager) Issue(request Request) (IssuedToken, error) {
	if m == nil {
		return IssuedToken{}, fmt.Errorf("confirmation manager is not initialized")
	}
	if m.ttl <= 0 {
		return IssuedToken{}, fmt.Errorf("confirmation TTL must be > 0")
	}

	token, err := randomToken()
	if err != nil {
		return IssuedToken{}, err
	}

	expiresAt := time.Now().Add(m.ttl)

	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanupLocked(time.Now())
	m.tokens[token] = tokenRecord{
		request:   request,
		expiresAt: expiresAt,
	}

	return IssuedToken{
		Token:     token,
		ExpiresAt: expiresAt,
	}, nil
}

func (m *Manager) Consume(token string, request Request) error {
	if m == nil {
		return fmt.Errorf("confirmation manager is not initialized")
	}
	if token == "" {
		return fmt.Errorf("confirmation token must not be empty")
	}

	now := time.Now()

	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanupLocked(now)

	record, ok := m.tokens[token]
	if !ok {
		return fmt.Errorf("confirmation token is invalid or expired")
	}

	if record.expiresAt.Before(now) {
		delete(m.tokens, token)
		return fmt.Errorf("confirmation token is invalid or expired")
	}
	if record.request != request {
		return fmt.Errorf("confirmation token does not match this operation")
	}

	delete(m.tokens, token)
	return nil
}

func (m *Manager) PendingCount() int {
	if m == nil {
		return 0
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanupLocked(time.Now())
	return len(m.tokens)
}

func (m *Manager) cleanupLocked(now time.Time) {
	for token, record := range m.tokens {
		if !record.expiresAt.After(now) {
			delete(m.tokens, token)
		}
	}
}

func randomToken() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate confirmation token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
