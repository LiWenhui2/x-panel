package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"
)

type Validator interface {
	Validate(context.Context, []byte) error
}

type JSONValidator struct{}

func (JSONValidator) Validate(_ context.Context, content []byte) error {
	var config struct {
		Inbounds []json.RawMessage `json:"inbounds"`
	}
	if err := json.Unmarshal(content, &config); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	if len(config.Inbounds) < 2 {
		return errors.New("configuration needs API and at least one business inbound")
	}
	return nil
}

type CommandValidator struct {
	Binary  string
	Timeout time.Duration
}

func (v CommandValidator) Validate(ctx context.Context, content []byte) error {
	if v.Binary == "" {
		return errors.New("xray binary is not configured")
	}
	file, err := os.CreateTemp("", "xpanel-config-*.json")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if _, err = file.Write(content); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	if v.Timeout <= 0 {
		v.Timeout = 10 * time.Second
	}
	commandCtx, cancel := context.WithTimeout(ctx, v.Timeout)
	defer cancel()
	output, err := exec.CommandContext(commandCtx, v.Binary, "run", "-test", "-config", name).CombinedOutput()
	if err != nil {
		return fmt.Errorf("xray validation failed: %w: %s", err, string(output))
	}
	return nil
}
