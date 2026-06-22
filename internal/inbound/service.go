package inbound

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"regexp"
	"strings"
	"time"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-5][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)

type PortOpener interface {
	Allow(context.Context, int) error
}

type TrafficReader interface {
	ReadAndReset(context.Context) (map[string]int64, error)
}

type Dependencies struct {
	PortOpener    PortOpener
	TrafficReader TrafficReader
}

type Service struct {
	repository   Repository
	dependencies Dependencies
}

func NewService(repository Repository, dependencies ...Dependencies) *Service {
	service := &Service{repository: repository}
	if len(dependencies) > 0 {
		service.dependencies = dependencies[0]
	}
	return service
}

func (s *Service) List(ctx context.Context) ([]Inbound, error) {
	items, err := s.repository.List(ctx)
	if err != nil {
		return nil, err
	}
	if s.dependencies.TrafficReader != nil {
		if usage, readErr := s.dependencies.TrafficReader.ReadAndReset(ctx); readErr == nil {
			for index := range items {
				delta := usage[items[index].Email]
				if delta > 0 {
					if err := s.repository.AddUsedBytes(ctx, items[index].ID, delta); err != nil {
						return nil, err
					}
					items[index].UsedBytes += delta
				}
			}
		}
	}
	for index := range items {
		normalizeUsage(&items[index])
	}
	return items, nil
}

func (s *Service) Create(ctx context.Context, input CreateInput) (Inbound, error) {
	if err := Validate(input); err != nil {
		return Inbound{}, err
	}
	if input.ExpiryTime == "" {
		input.ExpiryTime = DefaultExpiryTime
	}
	if s.dependencies.PortOpener != nil {
		if err := s.dependencies.PortOpener.Allow(ctx, input.Port); err != nil {
			return Inbound{}, fmt.Errorf("open firewall port %d: %w", input.Port, err)
		}
	}
	item := Inbound{
		Remark: strings.TrimSpace(input.Remark), Listen: input.Listen, Port: input.Port,
		Protocol: input.Protocol, Network: input.Network, Security: input.Security,
		ClientID: strings.ToLower(input.ClientID), Email: input.Email, Enabled: input.Enabled,
		TotalBytes: input.TotalBytes, ExpiryTime: input.ExpiryTime, AlterID: input.AlterID,
		Sniffing: input.Sniffing, WSPath: input.WSPath,
		TLSCertFile: input.TLSCertFile, TLSKeyFile: input.TLSKeyFile,
	}
	created, err := s.repository.Create(ctx, item)
	if err != nil {
		return Inbound{}, err
	}
	normalizeUsage(&created)
	return created, nil
}

func normalizeUsage(item *Inbound) {
	if item.ExpiryTime == "" {
		item.ExpiryTime = DefaultExpiryTime
	}
	if item.TotalBytes > 0 {
		item.RemainingBytes = item.TotalBytes - item.UsedBytes
		if item.RemainingBytes < 0 {
			item.RemainingBytes = 0
		}
	}
}

func Validate(input CreateInput) error {
	var problems []string
	if strings.TrimSpace(input.Remark) == "" {
		problems = append(problems, "remark is required")
	}
	if input.Listen != "" && net.ParseIP(input.Listen) == nil {
		problems = append(problems, "listen must be an IP address")
	}
	if input.Port < 1 || input.Port > 65535 {
		problems = append(problems, "port must be between 1 and 65535")
	}
	if input.Protocol != ProtocolVLESS && input.Protocol != ProtocolVMess {
		problems = append(problems, "protocol must be vless or vmess")
	}
	if input.Network != NetworkTCP && input.Network != NetworkWS {
		problems = append(problems, "network must be tcp or ws")
	}
	if input.Security != SecurityNone && input.Security != SecurityTLS {
		problems = append(problems, "security must be none or tls")
	}
	if !uuidPattern.MatchString(input.ClientID) {
		problems = append(problems, "clientId must be a valid UUID")
	}
	if _, err := mail.ParseAddress(input.Email); err != nil {
		problems = append(problems, "email must be valid")
	}
	if input.TotalBytes < 0 {
		problems = append(problems, "totalBytes cannot be negative")
	}
	if input.AlterID < 0 || input.AlterID > 65535 {
		problems = append(problems, "alterId must be between 0 and 65535")
	}
	if input.ExpiryTime != "" {
		if _, err := time.Parse(time.RFC3339, input.ExpiryTime); err != nil {
			problems = append(problems, "expiryTime must be RFC3339")
		}
	}
	if input.Network == NetworkWS && !strings.HasPrefix(input.WSPath, "/") {
		problems = append(problems, "wsPath must start with /")
	}
	if input.Security == SecurityTLS && (strings.TrimSpace(input.TLSCertFile) == "" || strings.TrimSpace(input.TLSKeyFile) == "") {
		problems = append(problems, "TLS certificate and key files are required")
	}
	if len(problems) > 0 {
		return fmt.Errorf("%w: %s", ErrInvalidInput, strings.Join(problems, "; "))
	}
	return nil
}

var ErrInvalidInput = errors.New("invalid input")
var ErrConflict = errors.New("inbound conflicts with an existing record")
