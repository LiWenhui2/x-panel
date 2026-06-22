package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type CommandTrafficReader struct {
	Binary  string
	Server  string
	Timeout time.Duration
}

type statsResponse struct {
	Stats []struct {
		Name  string `json:"name"`
		Value int64  `json:"value"`
	} `json:"stat"`
}

func (r CommandTrafficReader) ReadAndReset(ctx context.Context) (map[string]int64, error) {
	if r.Binary == "" {
		return nil, errors.New("xray binary is not configured")
	}
	server := r.Server
	if server == "" {
		server = "127.0.0.1:10085"
	}
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	output, err := exec.CommandContext(commandCtx, r.Binary, "api", "statsquery", "--server="+server, "-pattern", "user", "-reset=true").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("query xray traffic: %w: %s", err, string(output))
	}
	return parseTrafficStats(output)
}

func parseTrafficStats(content []byte) (map[string]int64, error) {
	var response statsResponse
	if err := json.Unmarshal(content, &response); err != nil {
		return nil, fmt.Errorf("decode xray traffic: %w", err)
	}
	usage := make(map[string]int64)
	for _, stat := range response.Stats {
		parts := strings.Split(stat.Name, ">>>")
		if len(parts) != 4 || parts[0] != "user" || parts[2] != "traffic" || stat.Value < 0 {
			continue
		}
		if parts[3] != "uplink" && parts[3] != "downlink" {
			continue
		}
		usage[parts[1]] += stat.Value
	}
	return usage, nil
}
