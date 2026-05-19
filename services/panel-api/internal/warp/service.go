package warp

import (
	"context"
	"crypto/rand"
	"encoding/base64"

	"golang.org/x/crypto/curve25519"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) GetForNode(ctx context.Context, nodeID string) (*Credentials, error) {
	return s.repo.Get(ctx, nodeID)
}

func (s *Service) SetForNode(ctx context.Context, input SetInput) (*Credentials, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}
	return s.repo.Upsert(ctx, input)
}

func (s *Service) DisableForNode(ctx context.Context, nodeID string) error {
	return s.repo.Delete(ctx, nodeID)
}

// GenerateCredentials generates a WireGuard keypair for WARP usage.
func (s *Service) GenerateCredentials() (*GenerateResult, error) {
	privateKey, publicKey, err := generateWireGuardKeypair()
	if err != nil {
		return nil, err
	}
	return &GenerateResult{PrivateKey: privateKey, PublicKey: publicKey}, nil
}

func generateWireGuardKeypair() (string, string, error) {
	var private [32]byte
	if _, err := rand.Read(private[:]); err != nil {
		return "", "", err
	}
	private[0] &= 248
	private[31] &= 127
	private[31] |= 64

	public, err := curve25519.X25519(private[:], curve25519.Basepoint)
	if err != nil {
		return "", "", err
	}

	return base64.StdEncoding.EncodeToString(private[:]), base64.StdEncoding.EncodeToString(public), nil
}
