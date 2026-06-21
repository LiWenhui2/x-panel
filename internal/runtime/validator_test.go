package runtime

import (
	"context"
	"testing"
)

func TestJSONValidator(t *testing.T) {
	valid := []byte(`{"inbounds":[{},{}]}`)
	if err := (JSONValidator{}).Validate(context.Background(), valid); err != nil {
		t.Fatal(err)
	}
	if err := (JSONValidator{}).Validate(context.Background(), []byte(`{"inbounds":[]}`)); err == nil {
		t.Fatal("expected error")
	}
}
