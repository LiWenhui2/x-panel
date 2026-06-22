package subscription

import "time"

type Subscription struct {
	ID         int64     `json:"id"`
	Name       string    `json:"name"`
	Enabled    bool      `json:"enabled"`
	InboundIDs []int64   `json:"inboundIds"`
	TokenHint  string    `json:"tokenHint"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type Input struct {
	Name       string  `json:"name"`
	Enabled    bool    `json:"enabled"`
	InboundIDs []int64 `json:"inboundIds"`
}
