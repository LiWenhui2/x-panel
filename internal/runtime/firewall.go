package runtime

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type CommandPortOpener struct {
	Command []string
	Timeout time.Duration
}

func (o CommandPortOpener) Allow(ctx context.Context, port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("invalid port: %d", port)
	}
	if len(o.Command) == 0 {
		return errors.New("firewall command is not configured")
	}
	timeout := o.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	portValue := strconv.Itoa(port)
	args := append([]string(nil), o.Command[1:]...)
	replaced := false
	for index := range args {
		if strings.Contains(args[index], "{port}") {
			args[index] = strings.ReplaceAll(args[index], "{port}", portValue)
			replaced = true
		}
	}
	if !replaced {
		args = append(args, portValue)
	}
	output, err := exec.CommandContext(commandCtx, o.Command[0], args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("firewall command failed: %w: %s", err, string(output))
	}
	return nil
}
