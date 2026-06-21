package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"
)

const hashRounds = 120_000

var (
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrInvalidInput       = errors.New("invalid auth input")
)

type User struct {
	ID           int64
	Username     string
	PasswordHash string
	Salt         string
	CreatedAt    time.Time
}

type Repository interface {
	HasUser(context.Context) (bool, error)
	ReplaceAdministrator(context.Context, string, string, string) error
	FindUserByUsername(context.Context, string) (User, error)
	CreateSession(context.Context, int64, string, time.Time) error
	FindSession(context.Context, string) (User, error)
	DeleteSession(context.Context, string) error
}

type Service struct{ repository Repository }

func NewService(repository Repository) *Service { return &Service{repository: repository} }

func (s *Service) NeedsSetup(ctx context.Context) (bool, error) {
	hasUser, err := s.repository.HasUser(ctx)
	return !hasUser, err
}

func (s *Service) Setup(ctx context.Context, username, password string) error {
	username, password, err := normalize(username, password)
	if err != nil {
		return err
	}
	hash, salt, err := HashPassword(password)
	if err != nil {
		return err
	}
	return s.repository.ReplaceAdministrator(ctx, username, hash, salt)
}

func (s *Service) Login(ctx context.Context, username, password string) (string, time.Time, error) {
	username = strings.TrimSpace(username)
	user, err := s.repository.FindUserByUsername(ctx, username)
	if err != nil {
		return "", time.Time{}, ErrInvalidCredentials
	}
	if !VerifyPassword(password, user.Salt, user.PasswordHash) {
		return "", time.Time{}, ErrInvalidCredentials
	}
	token, err := randomToken(32)
	if err != nil {
		return "", time.Time{}, err
	}
	expiresAt := time.Now().UTC().Add(7 * 24 * time.Hour)
	if err := s.repository.CreateSession(ctx, user.ID, token, expiresAt); err != nil {
		return "", time.Time{}, err
	}
	return token, expiresAt, nil
}

func (s *Service) CurrentUser(ctx context.Context, token string) (User, error) {
	if strings.TrimSpace(token) == "" {
		return User{}, ErrInvalidCredentials
	}
	user, err := s.repository.FindSession(ctx, token)
	if err != nil {
		return User{}, ErrInvalidCredentials
	}
	return user, nil
}

func (s *Service) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return s.repository.DeleteSession(ctx, token)
}

func normalize(username, password string) (string, string, error) {
	username = strings.TrimSpace(username)
	if len(username) < 3 || len(username) > 64 {
		return "", "", fmt.Errorf("%w: username must be 3-64 characters", ErrInvalidInput)
	}
	if len(password) < 8 || len(password) > 256 {
		return "", "", fmt.Errorf("%w: password must be 8-256 characters", ErrInvalidInput)
	}
	return username, password, nil
}

func HashPassword(password string) (string, string, error) {
	saltBytes := make([]byte, 16)
	if _, err := rand.Read(saltBytes); err != nil {
		return "", "", err
	}
	salt := base64.RawStdEncoding.EncodeToString(saltBytes)
	return derive(password, salt), salt, nil
}

func VerifyPassword(password, salt, expected string) bool {
	actual := derive(password, salt)
	return subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) == 1
}

func derive(password, salt string) string {
	sum := sha256.Sum256([]byte(salt + "\x00" + password))
	for i := 0; i < hashRounds; i++ {
		h := sha256.New()
		h.Write(sum[:])
		h.Write([]byte(password))
		h.Write([]byte(salt))
		copy(sum[:], h.Sum(nil))
	}
	return base64.RawStdEncoding.EncodeToString(sum[:])
}

func randomToken(size int) (string, error) {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}
