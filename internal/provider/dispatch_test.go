package provider

import (
	"context"
	"errors"
	"testing"
)

func TestForgejoAdapter_DispatchWorkflow_Unsupported(t *testing.T) {
	adapter := &ForgejoAdapter{}
	err := adapter.DispatchWorkflow(context.Background(), "token", "owner", "repo", "renovate.yml", "main", nil)
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("expected ErrUnsupported, got %v", err)
	}
}
