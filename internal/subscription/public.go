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
			Name: name, OriginalName: name, Remark: node.Remark, Protocol: node.Protocol, Address: nodeAddress, Port: node.Port,
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
	if item.Protocol == inbound.ProtocolVMess {
		payload := map[string]any{
			"v": "2", "ps": item.Remark, "add": address, "port": item.Port, "id": item.ClientID,
			"aid": item.AlterID, "net": item.Network, "type": "none", "host": "", "path": item.WSPath,
			"tls":    item.Security,
			"xpanel": backendMetadata(item, address),
		}
		content, _ := json.Marshal(payload)
		return "vmess://" + base64.StdEncoding.EncodeToString(content)
	}
	values := url.Values{}
	values.Set("type", string(item.Network))
	values.Set("security", string(item.Security))
	if item.WSPath != "" {
		values.Set("path", item.WSPath)
		values.Set("xpanel_path", item.WSPath)
	}
	for key, value := range backendMetadataStrings(item, address) {
		values.Set(key, value)
	}
	return fmt.Sprintf("vless://%s@%s:%d?%s#%s", item.ClientID, address, item.Port, values.Encode(), url.QueryEscape(item.Remark))
}

func backendMetadata(item inbound.Inbound, address string) map[string]any {
	return map[string]any{
		"name": item.Remark, "original_name": item.Remark, "remark": item.Remark,
		"protocol": item.Protocol, "address": address, "port": item.Port,
		"transport": item.Network, "security": item.Security, "sni": "", "host": "",
		"path": item.WSPath, "alpn": "", "email": item.Email,
		"credential": map[string]any{"uuid": item.ClientID, "alter_id": item.AlterID},
		"config":     map[string]any{"network": item.Network, "security": item.Security, "ws_path": item.WSPath},
		"expire_at":  item.ExpiryTime, "total_bytes": item.TotalBytes,
		"used_bytes": item.UsedBytes, "remain_bytes": item.RemainingBytes,
	}
}

func backendMetadataStrings(item inbound.Inbound, address string) map[string]string {
	return map[string]string{
		"xpanel_name":            item.Remark,
		"xpanel_original_name":   item.Remark,
		"xpanel_remark":          item.Remark,
		"xpanel_protocol":        string(item.Protocol),
		"xpanel_address":         address,
		"xpanel_port":            strconv.Itoa(item.Port),
		"xpanel_transport":       string(item.Network),
		"xpanel_security":        string(item.Security),
		"xpanel_sni":             "",
		"xpanel_host":            "",
		"xpanel_alpn":            "",
		"xpanel_email":           item.Email,
		"xpanel_expire_at":       item.ExpiryTime,
		"xpanel_expiry":          item.ExpiryTime,
		"xpanel_total_bytes":     strconv.FormatInt(item.TotalBytes, 10),
		"xpanel_used_bytes":      strconv.FormatInt(item.UsedBytes, 10),
		"xpanel_remain_bytes":    strconv.FormatInt(item.RemainingBytes, 10),
		"xpanel_remaining_bytes": strconv.FormatInt(item.RemainingBytes, 10),
	}
}

func BuildLinkList(item Subscription, nodes []inbound.Inbound, address string) []string {
	document := BuildPublicDocument(item, nodes, address)
	links := make([]string, 0, len(document.Nodes))
	for _, node := range document.Nodes {
		links = append(links, node.ShareLink)
	}
	return links
}

func BuildV2RaySubscription(item Subscription, nodes []inbound.Inbound, address string) string {
	return base64.StdEncoding.EncodeToString([]byte(strings.Join(BuildLinkList(item, nodes, address), "\n")))
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
