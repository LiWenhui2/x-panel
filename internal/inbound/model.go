package inbound

import "time"

const DefaultExpiryTime = "2099-12-31T23:59:59Z"

type Protocol string
type Network string
type Security string

const (
	ProtocolVLESS Protocol = "vless"
	ProtocolVMess Protocol = "vmess"
	NetworkTCP    Network  = "tcp"
	NetworkWS     Network  = "ws"
	SecurityNone  Security = "none"
	SecurityTLS   Security = "tls"
)

type Inbound struct {
	ID             int64     `json:"id"`
	Remark         string    `json:"remark"`
	Tag            string    `json:"tag"`
	Listen         string    `json:"listen"`
	Port           int       `json:"port"`
	Protocol       Protocol  `json:"protocol"`
	Network        Network   `json:"network"`
	Security       Security  `json:"security"`
	ClientID       string    `json:"clientId"`
	Email          string    `json:"email"`
	Enabled        bool      `json:"enabled"`
	TotalBytes     int64     `json:"totalBytes"`
	UsedBytes      int64     `json:"usedBytes"`
	RemainingBytes int64     `json:"remainingBytes"`
	ExpiryTime     string    `json:"expiryTime"`
	AlterID        int       `json:"alterId"`
	Sniffing       bool      `json:"sniffing"`
	WSPath         string    `json:"wsPath"`
	TLSCertFile    string    `json:"tlsCertFile"`
	TLSKeyFile     string    `json:"tlsKeyFile"`
	CreatedAt      time.Time `json:"createdAt"`
}

type CreateInput struct {
	Remark      string   `json:"remark"`
	Listen      string   `json:"listen"`
	Port        int      `json:"port"`
	Protocol    Protocol `json:"protocol"`
	Network     Network  `json:"network"`
	Security    Security `json:"security"`
	ClientID    string   `json:"clientId"`
	Email       string   `json:"email"`
	Enabled     bool     `json:"enabled"`
	TotalBytes  int64    `json:"totalBytes"`
	ExpiryTime  string   `json:"expiryTime"`
	AlterID     int      `json:"alterId"`
	Sniffing    bool     `json:"sniffing"`
	WSPath      string   `json:"wsPath"`
	TLSCertFile string   `json:"tlsCertFile"`
	TLSKeyFile  string   `json:"tlsKeyFile"`
}
