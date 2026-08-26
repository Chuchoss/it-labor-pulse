package assistant

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"sync"
	"time"
)

func LinkTokenHash(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

type Linker struct {
	mu     sync.Mutex
	tokens map[string]linkToken
	ttl    time.Duration
}
type linkToken struct {
	expires time.Time
	used    bool
}

func NewLinker(ttl time.Duration) *Linker {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	return &Linker{tokens: map[string]linkToken{}, ttl: ttl}
}

func (l *Linker) Issue(now time.Time) (string, error) {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	l.mu.Lock()
	l.tokens[token] = linkToken{expires: now.Add(l.ttl)}
	l.mu.Unlock()
	return token, nil
}

func (l *Linker) Consume(token string, now time.Time) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	item, ok := l.tokens[token]
	if !ok || item.used || !now.Before(item.expires) {
		return errors.New("invalid or expired link token")
	}
	item.used = true
	l.tokens[token] = item
	return nil
}
