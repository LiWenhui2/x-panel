package subscription

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"xpanel/internal/inbound"
)

type PublicNode struct {
	ID             int64            `json:"id"`
	Name           string           `json:"name"`
	OriginalName   string           `json:"originalName,omitempty"`
	Remark         string           `json:"remark"`
	Protocol       inbound.Protocol `json:"protocol"`
	Address        string           `json:"address"`
	Port           int              `json:"port"`
	Transport      inbound.Network  `json:"transport"`
	Network        inbound.Network  `json:"network"`
	Security       inbound.Security `json:"security"`
	SNI            string           `json:"sni,omitempty"`
	Host           string           `json:"host,omitempty"`
	ClientID       string           `json:"clientId"`
	Email          string           `json:"email"`
	AlterID        int              `json:"alterId"`
	ALPN           string           `json:"alpn,omitempty"`
	WSPath         string           `json:"wsPath,omitempty"`
	Path           string           `json:"path,omitempty"`
	TotalBytes     int64            `json:"totalBytes"`
	UsedBytes      int64            `json:"usedBytes"`
	RemainingBytes int64            `json:"remainingBytes"`
	ExpiryTime     string           `json:"expiryTime"`
	ShareLink      string           `json:"shareLink"`
}

type PublicDocument struct {
	Version        int          `json:"version"`
	Name           string       `json:"name"`
	GeneratedAt    time.Time    `json:"generatedAt"`
	TotalBytes     int64        `json:"totalBytes"`
	UsedBytes      int64        `json:"usedBytes"`
	RemainingBytes int64        `json:"remainingBytes"`
	ExpiryTime     string       `json:"expiryTime"`
	Nodes          []PublicNode `json:"nodes"`
}

func BuildPublicDocument(item Subscription, nodes []inbound.Inbound, address string) PublicDocument {
	document := PublicDocument{
		Version: itemVersion(), Name: item.Name, GeneratedAt: time.Now().UTC(),
		TotalBytes: item.TotalBytes, UsedBytes: item.UsedBytes, RemainingBytes: item.RemainingBytes,
		ExpiryTime: item.ExpiryTime, Nodes: []PublicNode{},
	}
	for _, node := range nodes {
		nodeAddress := strings.TrimSpace(node.Listen)
		if nodeAddress == "" || nodeAddress == "0.0.0.0" || nodeAddress == "::" || nodeAddress == "127.0.0.1" {
			nodeAddress = address
		}
		name := node.Remark
		if strings.TrimSpace(name) == "" {
			name = node.Tag
		}
		publicNode := PublicNode{
			ID: node.ID, Name: name, OriginalName: name, Remark: node.Remark, Protocol: node.Protocol, Address: nodeAddress, Port: node.Port,
			Transport: node.Network, Network: node.Network, Security: node.Security, ClientID: node.ClientID, Email: node.Email,
			AlterID: node.AlterID, WSPath: node.WSPath, Path: node.WSPath, TotalBytes: item.TotalBytes, UsedBytes: item.UsedBytes,
			RemainingBytes: item.RemainingBytes, ExpiryTime: item.ExpiryTime,
		}
		linkNode := node
		linkNode.TotalBytes, linkNode.UsedBytes, linkNode.RemainingBytes, linkNode.ExpiryTime = item.TotalBytes, item.UsedBytes, item.RemainingBytes, item.ExpiryTime
		publicNode.ShareLink = buildShareLink(linkNode, nodeAddress)
		document.Nodes = append(document.Nodes, publicNode)
	}
	return document
}

func itemVersion() int { return 2 }

func buildShareLink(item inbound.Inbound, address string) string {
	return buildShareLinkWithOptions(item, address, false)
}

func buildShadowrocketLink(item inbound.Inbound, address string) string {
	return buildShareLinkWithOptions(item, address, true)
}

func buildShareLinkWithOptions(item inbound.Inbound, address string, shadowrocket bool) string {
	if item.Protocol == inbound.ProtocolVMess {
		payload := map[string]any{
			"v": "2", "ps": item.Remark, "add": address, "port": item.Port, "id": item.ClientID,
			"aid": item.AlterID, "net": item.Network, "type": "none", "host": "", "path": item.WSPath,
			"tls": item.Security,
		}
		content, _ := json.Marshal(payload)
		return "vmess://" + base64.StdEncoding.EncodeToString(content)
	}
	values := url.Values{}
	values.Set("type", string(item.Network))
	values.Set("security", string(item.Security))
	if shadowrocket {
		values.Set("encryption", "none")
		if item.Network == inbound.NetworkTCP {
			values.Set("headerType", "none")
		}
	}
	if item.WSPath != "" {
		values.Set("path", item.WSPath)
	}
	return fmt.Sprintf("vless://%s@%s:%d?%s#%s", item.ClientID, address, item.Port, values.Encode(), url.QueryEscape(item.Remark))
}

type NexoraDocument struct {
	Version       int          `json:"version"`
	Client        string       `json:"client"`
	Type          string       `json:"type"`
	GeneratedAt   time.Time    `json:"generated_at"`
	Subscriptions []NexoraFeed `json:"subscriptions"`
	ProxyNodes    []NexoraNode `json:"proxy_nodes"`
}

type NexoraFeed struct {
	ID             int64     `json:"id"`
	UserID         int64     `json:"user_id"`
	Name           string    `json:"name"`
	URLCiphertext  string    `json:"url_ciphertext"`
	URLHash        string    `json:"url_hash"`
	Enabled        int       `json:"enabled"`
	TotalBytes     int64     `json:"total_bytes"`
	RemainBytes    int64     `json:"remain_bytes"`
	ExpireAt       string    `json:"expire_at"`
	LastUpdateTime time.Time `json:"last_update_time"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type NexoraNode struct {
	ID                   int64          `json:"id"`
	UserID               int64          `json:"user_id"`
	SubscriptionID       int64          `json:"subscription_id"`
	Name                 string         `json:"name"`
	OriginalName         string         `json:"original_name"`
	Remark               string         `json:"remark"`
	Protocol             string         `json:"protocol"`
	Address              string         `json:"address"`
	Port                 int            `json:"port"`
	Transport            string         `json:"transport"`
	Security             string         `json:"security"`
	SNI                  string         `json:"sni"`
	Host                 string         `json:"host"`
	Path                 string         `json:"path"`
	ALPN                 string         `json:"alpn"`
	CountryCode          string         `json:"country_code"`
	Region               string         `json:"region"`
	City                 string         `json:"city"`
	CredentialCiphertext string         `json:"credential_ciphertext"`
	Credential           map[string]any `json:"credential"`
	ConfigJSON           map[string]any `json:"config_json"`
	ShareLinkCiphertext  string         `json:"share_link_ciphertext"`
	ShareLink            string         `json:"share_link"`
	NodeHash             string         `json:"node_hash"`
	Enabled              int            `json:"enabled"`
	CreatedAt            time.Time      `json:"created_at"`
	UpdatedAt            time.Time      `json:"updated_at"`
}

func BuildLinkList(item Subscription, nodes []inbound.Inbound, address string) []string {
	document := BuildPublicDocument(item, nodes, address)
	links := make([]string, 0, len(document.Nodes))
	for _, node := range document.Nodes {
		links = append(links, node.ShareLink)
	}
	return links
}

func BuildNexoraSubscription(item Subscription, nodes []inbound.Inbound, address string) NexoraDocument {
	now := time.Now().UTC()
	document := BuildPublicDocument(item, nodes, address)
	enabled := 0
	if item.Enabled {
		enabled = 1
	}
	result := NexoraDocument{
		Version:     1,
		Client:      "Nexora",
		Type:        "subscription",
		GeneratedAt: now,
		Subscriptions: []NexoraFeed{{
			ID: item.ID, Name: item.Name, Enabled: enabled, TotalBytes: item.TotalBytes,
			RemainBytes: item.RemainingBytes, ExpireAt: item.ExpiryTime, LastUpdateTime: now,
			CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
		}},
		ProxyNodes: []NexoraNode{},
	}
	for _, node := range document.Nodes {
		result.ProxyNodes = append(result.ProxyNodes, NexoraNode{
			ID: node.ID, SubscriptionID: item.ID, Name: node.Name, OriginalName: node.OriginalName,
			Remark: node.Remark, Protocol: string(node.Protocol), Address: node.Address, Port: node.Port,
			Transport: string(node.Network), Security: string(node.Security), Path: node.WSPath,
			Credential: map[string]any{"uuid": node.ClientID, "email": node.Email, "alter_id": node.AlterID},
			ConfigJSON: map[string]any{
				"network": node.Network, "total_bytes": item.TotalBytes, "used_bytes": item.UsedBytes,
				"remain_bytes": item.RemainingBytes, "expire_at": item.ExpiryTime,
			},
			ShareLink: node.ShareLink, Enabled: 1, CreatedAt: now, UpdatedAt: now,
		})
	}
	return result
}

func BuildV2RaySubscription(item Subscription, nodes []inbound.Inbound, address string) string {
	return base64.StdEncoding.EncodeToString([]byte(strings.Join(BuildLinkList(item, nodes, address), "\n")))
}

func BuildShadowrocketSubscription(item Subscription, nodes []inbound.Inbound, address string) string {
	links := make([]string, 0, len(nodes))
	for _, node := range nodes {
		nodeAddress := strings.TrimSpace(node.Listen)
		if nodeAddress == "" || nodeAddress == "0.0.0.0" || nodeAddress == "::" || nodeAddress == "127.0.0.1" {
			nodeAddress = address
		}
		node.TotalBytes, node.UsedBytes, node.RemainingBytes, node.ExpiryTime = item.TotalBytes, item.UsedBytes, item.RemainingBytes, item.ExpiryTime
		links = append(links, buildShadowrocketLink(node, nodeAddress))
	}
	return base64.StdEncoding.EncodeToString([]byte(strings.Join(links, "\n")))
}

func BuildClashSubscription(item Subscription, nodes []inbound.Inbound, address string) string {
	document := BuildPublicDocument(item, nodes, address)
	var builder strings.Builder
	builder.WriteString("proxies:\n")
	names := make([]string, 0, len(document.Nodes))
	for _, node := range document.Nodes {
		names = append(names, node.Name)
		builder.WriteString(fmt.Sprintf("  - name: %q\n", node.Name))
		builder.WriteString(fmt.Sprintf("    type: %s\n", node.Protocol))
		builder.WriteString(fmt.Sprintf("    server: %q\n", node.Address))
		builder.WriteString(fmt.Sprintf("    port: %d\n", node.Port))
		builder.WriteString(fmt.Sprintf("    uuid: %s\n", node.ClientID))
		if node.Protocol == inbound.ProtocolVMess {
			builder.WriteString("    alterId: " + strconv.Itoa(node.AlterID) + "\n")
			builder.WriteString("    cipher: auto\n")
		}
		builder.WriteString(fmt.Sprintf("    tls: %t\n", node.Security == inbound.SecurityTLS))
		builder.WriteString(fmt.Sprintf("    network: %s\n", node.Network))
		if node.Network == inbound.NetworkWS {
			builder.WriteString("    ws-opts:\n")
			builder.WriteString(fmt.Sprintf("      path: %q\n", node.WSPath))
		}
	}
	builder.WriteString("proxy-groups:\n")
	builder.WriteString(fmt.Sprintf("  - name: %q\n", item.Name))
	builder.WriteString("    type: select\n")
	builder.WriteString("    proxies:\n")
	for _, name := range names {
		builder.WriteString(fmt.Sprintf("      - %q\n", name))
	}
	builder.WriteString("rules:\n  - MATCH," + strconv.Quote(item.Name) + "\n")
	return builder.String()
}

func BuildSingBoxSubscription(item Subscription, nodes []inbound.Inbound, address string) ([]byte, error) {
	document := BuildPublicDocument(item, nodes, address)
	outbounds := make([]map[string]any, 0, len(document.Nodes)+1)
	for _, node := range document.Nodes {
		outbound := map[string]any{
			"type": node.Protocol, "tag": node.Name, "server": node.Address,
			"server_port": node.Port, "uuid": node.ClientID,
		}
		if node.Protocol == inbound.ProtocolVMess {
			outbound["alter_id"] = node.AlterID
			outbound["security"] = "auto"
		}
		if node.Security == inbound.SecurityTLS {
			outbound["tls"] = map[string]any{"enabled": true}
		}
		if node.Network == inbound.NetworkWS {
			outbound["transport"] = map[string]any{"type": "ws", "path": node.WSPath}
		}
		outbounds = append(outbounds, outbound)
	}
	tags := make([]string, 0, len(document.Nodes))
	for _, node := range document.Nodes {
		tags = append(tags, node.Name)
	}
	outbounds = append(outbounds, map[string]any{"type": "selector", "tag": item.Name, "outbounds": tags})
	return json.MarshalIndent(map[string]any{
		"log":       map[string]any{"level": "warn"},
		"outbounds": outbounds,
	}, "", "  ")
}
