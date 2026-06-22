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
	Remark         string           `json:"remark"`
	Protocol       inbound.Protocol `json:"protocol"`
	Address        string           `json:"address"`
	Port           int              `json:"port"`
	Network        inbound.Network  `json:"network"`
	Security       inbound.Security `json:"security"`
	ClientID       string           `json:"clientId"`
	Email          string           `json:"email"`
	AlterID        int              `json:"alterId"`
	WSPath         string           `json:"wsPath,omitempty"`
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
	document := PublicDocument{Version: 1, Name: item.Name, GeneratedAt: time.Now().UTC(), Nodes: []PublicNode{}}
	unlimited := false
	var earliest time.Time
	for _, node := range nodes {
		nodeAddress := strings.TrimSpace(node.Listen)
		if nodeAddress == "" || nodeAddress == "0.0.0.0" || nodeAddress == "::" || nodeAddress == "127.0.0.1" {
			nodeAddress = address
		}
		publicNode := PublicNode{
			Remark: node.Remark, Protocol: node.Protocol, Address: nodeAddress, Port: node.Port,
			Network: node.Network, Security: node.Security, ClientID: node.ClientID, Email: node.Email,
			AlterID: node.AlterID, WSPath: node.WSPath, TotalBytes: node.TotalBytes, UsedBytes: node.UsedBytes,
			RemainingBytes: node.RemainingBytes, ExpiryTime: node.ExpiryTime,
		}
		publicNode.ShareLink = buildShareLink(node, nodeAddress)
		document.Nodes = append(document.Nodes, publicNode)
		document.UsedBytes += node.UsedBytes
		if node.TotalBytes == 0 {
			unlimited = true
		} else {
			document.TotalBytes += node.TotalBytes
			document.RemainingBytes += node.RemainingBytes
		}
		if expiry, err := time.Parse(time.RFC3339, node.ExpiryTime); err == nil && (earliest.IsZero() || expiry.Before(earliest)) {
			earliest = expiry
		}
	}
	if unlimited {
		document.TotalBytes, document.RemainingBytes = 0, 0
	}
	if !earliest.IsZero() {
		document.ExpiryTime = earliest.Format(time.RFC3339)
	}
	return document
}

func buildShareLink(item inbound.Inbound, address string) string {
	if item.Protocol == inbound.ProtocolVMess {
		payload := map[string]any{
			"v": "2", "ps": item.Remark, "add": address, "port": item.Port, "id": item.ClientID,
			"aid": item.AlterID, "net": item.Network, "type": "none", "host": "", "path": item.WSPath,
			"tls": item.Security,
			"xpanel": map[string]any{"email": item.Email, "expiryTime": item.ExpiryTime, "totalBytes": item.TotalBytes,
				"usedBytes": item.UsedBytes, "remainingBytes": item.RemainingBytes},
		}
		content, _ := json.Marshal(payload)
		return "vmess://" + base64.StdEncoding.EncodeToString(content)
	}
	values := url.Values{}
	values.Set("type", string(item.Network))
	values.Set("security", string(item.Security))
	if item.WSPath != "" {
		values.Set("path", item.WSPath)
	}
	values.Set("xpanel_email", item.Email)
	values.Set("xpanel_expiry", item.ExpiryTime)
	values.Set("xpanel_total_bytes", strconv.FormatInt(item.TotalBytes, 10))
	values.Set("xpanel_used_bytes", strconv.FormatInt(item.UsedBytes, 10))
	values.Set("xpanel_remaining_bytes", strconv.FormatInt(item.RemainingBytes, 10))
	return fmt.Sprintf("vless://%s@%s:%d?%s#%s", item.ClientID, address, item.Port, values.Encode(), url.QueryEscape(item.Remark))
}
