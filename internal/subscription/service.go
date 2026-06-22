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

	"xpanel/internal/inbound"
)

var (
	ErrInvalidInput = errors.New("invalid subscription input")
	ErrNotFound     = errors.New("subscription not found")
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
	return s.repository.ListSubscriptions(ctx)
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
	}, tokenHash)
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
	return updated, nil
}

func (s *Service) Rotate(ctx context.Context, id int64) (Subscription, string, error) {
	token, tokenHash, hint, err := newToken()
	if err != nil {
		return Subscription{}, "", err
	}
	updated, err := s.repository.RotateSubscriptionToken(ctx, id, tokenHash, hint)
	if err != nil {
		return Subscription{}, "", mapNotFound(err)
	}
	return updated, token, nil
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	return mapNotFound(s.repository.DeleteSubscription(ctx, id))
}

func (s *Service) Resolve(ctx context.Context, token string) (Subscription, []inbound.Inbound, error) {
	if strings.TrimSpace(token) == "" {
		return Subscription{}, nil, ErrNotFound
	}
	item, err := s.repository.FindSubscriptionByTokenHash(ctx, hashToken(token))
	if err != nil || !item.Enabled {
		return Subscription{}, nil, ErrNotFound
	}
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
