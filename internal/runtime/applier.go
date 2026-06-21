package runtime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type ApplyResult struct {
	ConfigPath string `json:"configPath"`
	SHA256     string `json:"sha256"`
	Output     string `json:"output,omitempty"`
}

type FileApplier struct {
	ConfigPath    string
	Validator     Validator
	ReloadCommand []string
	Timeout       time.Duration
}

func (a FileApplier) Apply(ctx context.Context, content []byte, sha256 string) (ApplyResult, error) {
	if a.ConfigPath == "" {
		return ApplyResult{}, errors.New("xray config path is not configured")
	}
	if a.Validator != nil {
		if err := a.Validator.Validate(ctx, content); err != nil {
			return ApplyResult{}, err
		}
	}
	directory := filepath.Dir(a.ConfigPath)
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return ApplyResult{}, fmt.Errorf("create xray config directory: %w", err)
	}
	temp, err := os.CreateTemp(directory, ".config-*.json")
	if err != nil {
		return ApplyResult{}, fmt.Errorf("create temporary xray config: %w", err)
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if _, err = temp.Write(content); err != nil {
		temp.Close()
		return ApplyResult{}, fmt.Errorf("write temporary xray config: %w", err)
	}
	if err = temp.Chmod(0o640); err != nil {
		temp.Close()
		return ApplyResult{}, fmt.Errorf("chmod temporary xray config: %w", err)
	}
	if err = temp.Close(); err != nil {
		return ApplyResult{}, fmt.Errorf("close temporary xray config: %w", err)
	}
	if err = os.Rename(tempName, a.ConfigPath); err != nil {
		return ApplyResult{}, fmt.Errorf("publish xray config: %w", err)
	}
	output, err := a.reload(ctx)
	if err != nil {
		return ApplyResult{}, err
	}
	return ApplyResult{ConfigPath: a.ConfigPath, SHA256: sha256, Output: output}, nil
}

func (a FileApplier) reload(ctx context.Context) (string, error) {
	if len(a.ReloadCommand) == 0 {
		return "", errors.New("xray reload command is not configured")
	}
	timeout := a.Timeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	command := exec.CommandContext(commandCtx, a.ReloadCommand[0], a.ReloadCommand[1:]...)
	output, err := command.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("reload xray service failed: %w: %s", err, string(output))
	}
	return string(output), nil
}
