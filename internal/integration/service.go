package integration

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

var ErrInvalidSettings = errors.New("invalid integration settings")

type Options struct {
	AllowedIPs []string
	Token      string
}

type Settings struct {
	AllowedIPs      []string `json:"allowedIps"`
	TokenConfigured bool     `json:"tokenConfigured"`
	TokenHint       string   `json:"tokenHint"`
}

type UpdateInput struct {
	AllowedIPs  []string `json:"allowedIps"`
	RotateToken bool     `json:"rotateToken"`
}

type storedSettings struct {
	AllowedIPs []string  `json:"allowedIps"`
	TokenHash  string    `json:"tokenHash"`
	TokenHint  string    `json:"tokenHint"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type Service struct {
	mu       sync.RWMutex
	path     string
	settings storedSettings
}

func Open(path string, bootstrap Options) (*Service, error) {
	service := &Service{path: path}
	payload, err := os.ReadFile(path)
	if err == nil {
		if err := json.Unmarshal(payload, &service.settings); err != nil {
			return nil, err
		}
		allowed, err := normalizeAllowedIPs(service.settings.AllowedIPs)
		if err != nil {
			return nil, err
		}
		service.settings.AllowedIPs = allowed
		return service, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	allowed, err := normalizeAllowedIPs(bootstrap.AllowedIPs)
	if err != nil {
		return nil, err
	}
	service.settings.AllowedIPs = allowed
	if token := strings.TrimSpace(bootstrap.Token); token != "" {
		service.settings.TokenHash = hashToken(token)
		service.settings.TokenHint = tokenHint(token)
	}
	if len(allowed) > 0 || service.settings.TokenHash != "" {
		service.settings.UpdatedAt = time.Now().UTC()
		if err := service.persistLocked(); err != nil {
			return nil, err
		}
	}
	return service, nil
}

func (s *Service) Current() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentLocked()
}

func (s *Service) Update(input UpdateInput) (Settings, string, error) {
	allowed, err := normalizeAllowedIPs(input.AllowedIPs)
	if err != nil {
		return Settings{}, "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.settings.AllowedIPs = allowed
	plainToken := ""
	if input.RotateToken {
		plainToken, err = generateToken()
		if err != nil {
			return Settings{}, "", err
		}
		s.settings.TokenHash = hashToken(plainToken)
		s.settings.TokenHint = tokenHint(plainToken)
	}
	s.settings.UpdatedAt = time.Now().UTC()
	if err := s.persistLocked(); err != nil {
		return Settings{}, "", err
	}
	return s.currentLocked(), plainToken, nil
}

func (s *Service) Authorize(remoteAddress, token string) bool {
	token = strings.TrimSpace(token)
	if token == "" {
		return false
	}
	s.mu.RLock()
	settings := s.settings
	s.mu.RUnlock()
	if settings.TokenHash == "" || len(settings.AllowedIPs) == 0 {
		return false
	}
	want, err := hex.DecodeString(settings.TokenHash)
	if err != nil {
		return false
	}
	got := sha256.Sum256([]byte(token))
	if len(want) != len(got) || subtle.ConstantTimeCompare(want, got[:]) != 1 {
		return false
	}
	ip := remoteIP(remoteAddress)
	if ip == nil {
		return false
	}
	for _, allowed := range settings.AllowedIPs {
		if candidate := net.ParseIP(allowed); candidate != nil && candidate.Equal(ip) {
			return true
		}
		if _, network, parseErr := net.ParseCIDR(allowed); parseErr == nil && network.Contains(ip) {
			return true
		}
	}
	return false
}

func (s *Service) currentLocked() Settings {
	return Settings{
		AllowedIPs:      append([]string(nil), s.settings.AllowedIPs...),
		TokenConfigured: s.settings.TokenHash != "",
		TokenHint:       s.settings.TokenHint,
	}
}

func (s *Service) persistLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o750); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(s.settings, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(s.path, append(payload, '\n'), 0o600); err != nil {
		return err
	}
	return os.Chmod(s.path, 0o600)
}

func normalizeAllowedIPs(values []string) ([]string, error) {
	unique := map[string]struct{}{}
	for _, raw := range values {
		for _, value := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ';' || r == '\n' || r == '\r' }) {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			if ip := net.ParseIP(value); ip != nil {
				unique[ip.String()] = struct{}{}
				continue
			}
			if _, network, err := net.ParseCIDR(value); err == nil {
				unique[network.String()] = struct{}{}
				continue
			}
			return nil, fmt.Errorf("%w: invalid IP or CIDR %s", ErrInvalidSettings, value)
		}
	}
	result := make([]string, 0, len(unique))
	for value := range unique {
		result = append(result, value)
	}
	sort.Strings(result)
	return result, nil
}

func remoteIP(address string) net.IP {
	if host, _, err := net.SplitHostPort(strings.TrimSpace(address)); err == nil {
		return net.ParseIP(host)
	}
	return net.ParseIP(strings.Trim(strings.TrimSpace(address), "[]"))
}

func generateToken() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return "xp_" + base64.RawURLEncoding.EncodeToString(value), nil
}

func hashToken(token string) string {
	value := sha256.Sum256([]byte(token))
	return hex.EncodeToString(value[:])
}

func tokenHint(token string) string {
	if len(token) <= 8 {
		return token
	}
	return token[:3] + "..." + token[len(token)-5:]
}
