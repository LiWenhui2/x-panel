package runtime

import (
	"context"
	"testing"
)

func TestCommandPortOpenerRejectsInvalidPort(t *testing.T) {
	opener := CommandPortOpener{Command: []string{"unused"}}
	for _, port := range []int{0, 65536} {
		if err := opener.Allow(context.Background(), port); err == nil {
			t.Fatalf("expected port %d to be rejected", port)
		}
	}
}
