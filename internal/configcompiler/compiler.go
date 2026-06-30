package configcompiler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

	"xpanel/internal/inbound"
)

type Result struct {
	Content []byte `json:"-"`
	SHA256  string `json:"sha256"`
}

type Compiler struct{ APIPort int }

func New() *Compiler { return &Compiler{APIPort: 10085} }

func (c *Compiler) Compile(items []inbound.Inbound) (Result, error) {
	sorted := append([]inbound.Inbound(nil), items...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })
	ports := map[int]bool{c.APIPort: true}
	inbounds := []any{map[string]any{
		"tag": "api", "listen": "127.0.0.1", "port": c.APIPort, "protocol": "dokodemo-door",
		"settings": map[string]any{"address": "127.0.0.1"},
	}}
	blockedInboundTags := []string{}
	for _, item := range sorted {
		if !item.Enabled && !item.TrafficBlocked {
			continue
		}
		if ports[item.Port] {
			return Result{}, fmt.Errorf("duplicate or reserved port: %d", item.Port)
		}
		ports[item.Port] = true
		stream := map[string]any{"network": item.Network, "security": item.Security}
		if item.Network == inbound.NetworkWS {
			stream["wsSettings"] = map[string]any{"path": item.WSPath}
		}
		if item.Security == inbound.SecurityTLS {
			stream["tlsSettings"] = map[string]any{"certificates": []any{map[string]any{
				"certificateFile": item.TLSCertFile,
				"keyFile":         item.TLSKeyFile,
			}}}
		}
		settings := map[string]any{"clients": []any{map[string]any{
			"id": item.ClientID, "email": item.Email,
		}}}
		if item.Protocol == inbound.ProtocolVLESS {
			settings["decryption"] = "none"
		} else {
			settings["clients"] = []any{map[string]any{
				"id": item.ClientID, "email": item.Email, "alterId": item.AlterID,
			}}
		}
		compiled := map[string]any{
			"tag": item.Tag, "listen": item.Listen, "port": item.Port, "protocol": item.Protocol,
			"settings": settings, "streamSettings": stream,
		}
		if item.Sniffing {
			compiled["sniffing"] = map[string]any{"enabled": true, "destOverride": []string{"http", "tls", "quic"}}
		}
		inbounds = append(inbounds, compiled)
		if item.TrafficBlocked {
			blockedInboundTags = append(blockedInboundTags, item.Tag)
		}
	}
	rules := []any{map[string]any{"type": "field", "inboundTag": []string{"api"}, "outboundTag": "api"}}
	if len(blockedInboundTags) > 0 {
		rules = append(rules, map[string]any{
			"type": "field", "inboundTag": blockedInboundTags, "outboundTag": "blocked",
		})
	}
	config := map[string]any{
		"log":   map[string]any{"loglevel": "warning"},
		"api":   map[string]any{"tag": "api", "services": []string{"HandlerService", "LoggerService", "StatsService"}},
		"stats": map[string]any{},
		"policy": map[string]any{
			"levels": map[string]any{"0": map[string]any{"statsUserUplink": true, "statsUserDownlink": true}},
			"system": map[string]any{"statsInboundUplink": true, "statsInboundDownlink": true},
		},
		"inbounds":  inbounds,
		"outbounds": []any{map[string]any{"tag": "direct", "protocol": "freedom"}, map[string]any{"tag": "blocked", "protocol": "blackhole"}},
		"routing":   map[string]any{"rules": rules},
	}
	content, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return Result{}, err
	}
	content = append(content, '\n')
	digest := sha256.Sum256(content)
	return Result{Content: content, SHA256: hex.EncodeToString(digest[:])}, nil
}
