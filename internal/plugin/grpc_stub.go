package plugin

import (
    "context"
    "errors"
)

// ErrGRPCNotImplemented is returned by GRPCStub methods until the gRPC
// transport layer is actually implemented.
var ErrGRPCNotImplemented = errors.New("plugin: gRPC not implemented yet")

// GRPCPlugin defines the interface for plugins that communicate via gRPC.
// This is a reserved interface for future use — the gRPC transport layer
// will be implemented when cross-language plugin support is needed.
type GRPCPlugin interface {
    Plugin
    // Capability returns the plugin's capability type (e.g. "deployer",
    // "translator", "seo_checker").
    Capability() string
    // Call invokes a remote method on the plugin.
    // Currently returns ErrGRPCNotImplemented.
    Call(ctx context.Context, method string, payload []byte) ([]byte, error)
    // Health checks whether the remote plugin is alive.
    Health(ctx context.Context) error
}

// GRPCStub is a placeholder for future gRPC-based plugins. It implements
// GRPCPlugin with stub methods that return ErrGRPCNotImplemented. The
// actual gRPC client will be implemented later in internal/plugin/grpc/.
type GRPCStub struct {
    name       string
    capability string
    address    string // remote gRPC address, e.g. "localhost:50051"
}

// NewGRPCStub creates a new GRPCStub. All methods return stub values
// until the gRPC transport layer is implemented.
func NewGRPCStub(name, capability, address string) *GRPCStub {
    return &GRPCStub{
        name:       name,
        capability: capability,
        address:    address,
    }
}

func (s *GRPCStub) Name() string { return s.name }

func (s *GRPCStub) Capability() string { return s.capability }

func (s *GRPCStub) Call(_ context.Context, _ string, _ []byte) ([]byte, error) {
    return nil, ErrGRPCNotImplemented
}

func (s *GRPCStub) Health(_ context.Context) error {
    return nil
}