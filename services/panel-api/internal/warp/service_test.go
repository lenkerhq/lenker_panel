package warp

import (
	"context"
	"encoding/base64"
	"testing"
)

type memRepo struct {
	data map[string]*Credentials
}

func newMemRepo() *memRepo { return &memRepo{data: make(map[string]*Credentials)} }

func (m *memRepo) Get(_ context.Context, nodeID string) (*Credentials, error) {
	c, ok := m.data[nodeID]
	if !ok {
		return nil, ErrNotFound
	}
	return c, nil
}

func (m *memRepo) Upsert(_ context.Context, input SetInput) (*Credentials, error) {
	endpoint := input.Endpoint
	if endpoint == "" {
		endpoint = "engage.cloudflareclient.com:2408"
	}
	c := &Credentials{
		NodeID: input.NodeID, PrivateKey: input.PrivateKey,
		PublicKey: input.PublicKey, Address: input.Address,
		Endpoint: endpoint, Enabled: true,
	}
	m.data[input.NodeID] = c
	return c, nil
}

func (m *memRepo) Delete(_ context.Context, nodeID string) error {
	if _, ok := m.data[nodeID]; !ok {
		return ErrNotFound
	}
	delete(m.data, nodeID)
	return nil
}

func TestServiceSetValidation(t *testing.T) {
	svc := NewService(newMemRepo())
	ctx := context.Background()

	_, err := svc.SetForNode(ctx, SetInput{NodeID: "n1", PrivateKey: "", PublicKey: "pub", Address: "addr"})
	if err != ErrInvalidInput {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}

	_, err = svc.SetForNode(ctx, SetInput{NodeID: "n1", PrivateKey: "priv", PublicKey: "", Address: "addr"})
	if err != ErrInvalidInput {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestServiceSetAndGet(t *testing.T) {
	svc := NewService(newMemRepo())
	ctx := context.Background()

	creds, err := svc.SetForNode(ctx, SetInput{NodeID: "n1", PrivateKey: "priv", PublicKey: "pub", Address: "10.0.0.1/32"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creds.Endpoint != "engage.cloudflareclient.com:2408" {
		t.Fatalf("expected default endpoint, got %s", creds.Endpoint)
	}

	got, err := svc.GetForNode(ctx, "n1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.PublicKey != "pub" {
		t.Fatalf("unexpected public key: %s", got.PublicKey)
	}
}

func TestServiceDisable(t *testing.T) {
	svc := NewService(newMemRepo())
	ctx := context.Background()

	svc.SetForNode(ctx, SetInput{NodeID: "n1", PrivateKey: "priv", PublicKey: "pub", Address: "10.0.0.1/32"})
	if err := svc.DisableForNode(ctx, "n1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err := svc.GetForNode(ctx, "n1")
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound after disable, got %v", err)
	}
}

func TestServiceDisableNotFound(t *testing.T) {
	svc := NewService(newMemRepo())
	err := svc.DisableForNode(context.Background(), "nonexistent")
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestServiceGenerateCredentials(t *testing.T) {
	svc := NewService(newMemRepo())
	result, err := svc.GenerateCredentials()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.PrivateKey == "" || result.PublicKey == "" {
		t.Fatalf("expected non-empty keys")
	}
	// Verify base64 encoding
	priv, err := base64.StdEncoding.DecodeString(result.PrivateKey)
	if err != nil || len(priv) != 32 {
		t.Fatalf("invalid private key encoding")
	}
	pub, err := base64.StdEncoding.DecodeString(result.PublicKey)
	if err != nil || len(pub) != 32 {
		t.Fatalf("invalid public key encoding")
	}
}
