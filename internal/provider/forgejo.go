package provider

import (
	"context"
	"time"

	"gitlens/internal/forgejo"
	ghclient "gitlens/internal/github"
)

// ForgejoAdapter wraps a *forgejo.Client so it satisfies the Provider
// interface. Forgejo-specific extras (instance URL picker) are exposed
// via the wrapper rather than the interface, so callers that need them
// can type-assert.
type ForgejoAdapter struct {
	*forgejo.Client
}

func NewForgejoAdapter(c *forgejo.Client) *ForgejoAdapter {
	return &ForgejoAdapter{Client: c}
}

func (a *ForgejoAdapter) Name() string { return "forgejo" }

// All other methods are inherited from *forgejo.Client and already
// match the Provider interface. The compile-time check below
// confirms it.
var _ Provider = (*ForgejoAdapter)(nil)

// Static interface check on *forgejo.Client itself (in case someone
// uses the concrete type directly).
var (
	_ Provider = (*forgejo.Client)(nil)
	_          = context.Background
	_          = time.Now
	_          = (*ghclient.User)(nil)
)
