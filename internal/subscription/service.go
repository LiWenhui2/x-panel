package subscription

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"xpanel/internal/inbound"
)

var (
	ErrInvalidInput     = errors.New("invalid subscription input")
	ErrNotFound         = errors.New("subscription not found")
	ErrInactive         = errors.New("subscription inactive")
	ErrTokenUnavailable = errors.New("subscription token unavailable")
)

type InboundSource interface {
	List(context.Context) ([]inbound.Inbound, error)
}

type Service struct {
	repository Repository
	inbounds   InboundSource
}

func NewService(repository Repository, inbounds InboundSource) *Service {
	return &Service{repository: repository, inbounds: inbounds}
}

func (s *Service) List(ctx context.Context) ([]Subscription, error) {
	items, err := s.repository.ListSubscriptions(ctx)
	if err != nil {
		return nil, err
	}
	for index := range items {
		normalizeUsage(&items[index])
	}
	return items, nil
}

func (s *Service) Create(ctx context.Context, input Input) (Subscription, string, error) {
	input, err := s.validate(ctx, input)
	if err != nil {
		return Subscription{}, "", err
	}
	token, tokenHash, hint, err := newToken()
	if err != nil {
		return Subscription{}, "", err
	}
	created, err := s.repository.CreateSubscription(ctx, Subscription{
		Name: input.Name, Enabled: input.Enabled, InboundIDs: input.InboundIDs, TokenHint: hint,
		Token: token, TotalBytes: input.TotalBytes, ExpiryTime: input.ExpiryTime,
	}, tokenHash)
	normalizeUsage(&created)
	return created, token, err
}

func (s *Service) Update(ctx context.Context, id int64, input Input) (Subscription, error) {
	input, err := s.validate(ctx, input)
	if err != nil {
		return Subscription{}, err
	}
	updated, err := s.repository.UpdateSubscription(ctx, id, input)
	if err != nil {
		return Subscription{}, mapNotFound(err)
	}
	normalizeUsage(&updated)
	return updated, nil
}

func (s *Service) Renew(ctx context.Context, id int64, input RenewInput) (Subscription, error) {
	if id <= 0 {
		return Subscription{}, ErrNotFound
	}
	if input.Days < 1 || input.Days > 3650 {
		return Subscription{}, fmt.Errorf("%w: renewal days must be between 1 and 3650", ErrInvalidInput)
	}
	items, err := s.repository.ListSubscriptions(ctx)
	if err != nil {
		return Subscription{}, err
	}
	var current *Subscription
	for index := range items {
		if items[index].ID == id {
			current = &items[index]
			break
		}
	}
	if current == nil {
		return Subscription{}, ErrNotFound
	}
	base := time.Now().UTC()
	if expiry, parseErr := time.Parse(time.RFC3339, current.ExpiryTime); parseErr == nil && expiry.After(base) {
		base = expiry
	}
	updated, err := s.repository.UpdateSubscription(ctx, id, Input{
		Name: current.Name, Enabled: true, InboundIDs: current.InboundIDs,
		TotalBytes: current.TotalBytes, ExpiryTime: base.AddDate(0, 0, input.Days).Format(time.RFC3339),
	})
	if err != nil {
		return Subscription{}, mapNotFound(err)
	}
	normalizeUsage(&updated)
	return updated, nil
}

func (s *Service) Rotate(ctx context.Context, id int64) (Subscription, string, error) {
	token, tokenHash, hint, err := newToken()
	if err != nil {
		return Subscription{}, "", err
	}
	updated, err := s.repository.RotateSubscriptionToken(ctx, id, tokenHash, hint, token)
	if err != nil {
		return Subscription{}, "", mapNotFound(err)
	}
	normalizeUsage(&updated)
	return updated, token, nil
}

func (s *Service) Token(ctx context.Context, id int64) (Subscription, string, error) {
	if id <= 0 {
		return Subscription{}, "", ErrNotFound
	}
	item, token, err := s.repository.SubscriptionToken(ctx, id)
	if err != nil {
		return Subscription{}, "", mapNotFound(err)
	}
	if strings.TrimSpace(token) == "" {
		return Subscription{}, "", ErrTokenUnavailable
	}
	normalizeUsage(&item)
	return item, token, nil
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	return mapNotFound(s.repository.DeleteSubscription(ctx, id))
}

func (s *Service) Resolve(ctx context.Context, token string) (Subscription, []inbound.Inbound, error) {
	if strings.TrimSpace(token) == "" {
		return Subscription{}, nil, ErrNotFound
	}
	item, err := s.repository.FindSubscriptionByTokenHash(ctx, hashToken(token))
	if err != nil {
		return Subscription{}, nil, ErrNotFound
	}
	if !subscriptionActive(item, time.Now().UTC()) {
		return Subscription{}, nil, ErrInactive
	}
	normalizeUsage(&item)
	all, err := s.inbounds.List(ctx)
	if err != nil {
		return Subscription{}, nil, err
	}
	wanted := make(map[int64]bool, len(item.InboundIDs))
	for _, id := range item.InboundIDs {
		wanted[id] = true
	}
	selected := make([]inbound.Inbound, 0, len(wanted))
	for _, node := range all {
		if wanted[node.ID] && node.Enabled {
			selected = append(selected, node)
		}
	}
	return item, selected, nil
}

func (s *Service) validate(ctx context.Context, input Input) (Input, error) {
	input.Name = strings.TrimSpace(input.Name)
	if len(input.Name) < 1 || len(input.Name) > 100 {
		return Input{}, fmt.Errorf("%w: name must be 1-100 characters", ErrInvalidInput)
	}
	if len(input.InboundIDs) == 0 {
		return Input{}, fmt.Errorf("%w: select at least one inbound", ErrInvalidInput)
	}
	if input.TotalBytes < 0 {
		return Input{}, fmt.Errorf("%w: totalBytes cannot be negative", ErrInvalidInput)
	}
	if input.ExpiryTime == "" {
		input.ExpiryTime = inbound.DefaultExpiryTime
	}
	if _, err := time.Parse(time.RFC3339, input.ExpiryTime); err != nil {
		return Input{}, fmt.Errorf("%w: expiryTime must be RFC3339", ErrInvalidInput)
	}
	seen := map[int64]bool{}
	for _, id := range input.InboundIDs {
		if id <= 0 || seen[id] {
			return Input{}, fmt.Errorf("%w: inbound IDs must be unique positive integers", ErrInvalidInput)
		}
		seen[id] = true
	}
	all, err := s.inbounds.List(ctx)
	if err != nil {
		return Input{}, err
	}
	available := map[int64]bool{}
	for _, node := range all {
		available[node.ID] = true
	}
	for id := range seen {
		if !available[id] {
			return Input{}, fmt.Errorf("%w: inbound %d does not exist", ErrInvalidInput, id)
		}
	}
	sort.Slice(input.InboundIDs, func(i, j int) bool { return input.InboundIDs[i] < input.InboundIDs[j] })
	return input, nil
}

func normalizeUsage(item *Subscription) {
	if item.ExpiryTime == "" {
		item.ExpiryTime = inbound.DefaultExpiryTime
	}
	if item.TotalBytes <= 0 {
		item.RemainingBytes = 0
		return
	}
	item.RemainingBytes = item.TotalBytes - item.UsedBytes
	if item.RemainingBytes < 0 {
		item.RemainingBytes = 0
	}
}

func subscriptionActive(item Subscription, now time.Time) bool {
	if !item.Enabled {
		return false
	}
	if item.TotalBytes <= 0 || item.UsedBytes >= item.TotalBytes {
		return false
	}
	if expiry, err := time.Parse(time.RFC3339, item.ExpiryTime); err == nil && !now.Before(expiry) {
		return false
	}
	return true
}

func newToken() (string, string, string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", "", "", err
	}
	token := base64.RawURLEncoding.EncodeToString(bytes)
	return token, hashToken(token), token[len(token)-8:], nil
}

func hashToken(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}

func mapNotFound(err error) error {
	if errors.Is(err, ErrNotFound) {
		return ErrNotFound
	}
	return err
}
