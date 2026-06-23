package subscription

import "time"

type Subscription struct {
	ID             int64     `json:"id"`
	Name           string    `json:"name"`
	Enabled        bool      `json:"enabled"`
	InboundIDs     []int64   `json:"inboundIds"`
	TokenHint      string    `json:"tokenHint"`
	TotalBytes     int64     `json:"totalBytes"`
	UsedBytes      int64     `json:"usedBytes"`
	RemainingBytes int64     `json:"remainingBytes"`
	ExpiryTime     string    `json:"expiryTime"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type Input struct {
	Name       string  `json:"name"`
	Enabled    bool    `json:"enabled"`
	InboundIDs []int64 `json:"inboundIds"`
	TotalBytes int64   `json:"totalBytes"`
	ExpiryTime string  `json:"expiryTime"`
}
