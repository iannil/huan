package plugin

import (
    "context"
    "testing"
)

func TestGRPCStub_ImplementsPlugin(t *testing.T) {
    s := NewGRPCStub("test-plugin", "deployer", "localhost:50051")
    if s.Name() != "test-plugin" {
        t.Errorf("Name() = %q, want test-plugin", s.Name())
    }
}

func TestGRPCStub_Capability(t *testing.T) {
    s := NewGRPCStub("test", "deployer", "")
    if s.Capability() != "deployer" {
        t.Errorf("Capability() = %q, want deployer", s.Capability())
    }
}

func TestGRPCStub_CallReturnsNotImplemented(t *testing.T) {
    s := NewGRPCStub("test", "", "")
    _, err := s.Call(context.Background(), "Deploy", nil)
    if err != ErrGRPCNotImplemented {
        t.Errorf("Call: want ErrGRPCNotImplemented, got %v", err)
    }
}

func TestGRPCStub_HealthReturnsNil(t *testing.T) {
    s := NewGRPCStub("test", "", "")
    err := s.Health(context.Background())
    if err != nil {
        t.Errorf("Health: want nil, got %v", err)
    }
}