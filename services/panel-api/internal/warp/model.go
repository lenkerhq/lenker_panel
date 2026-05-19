package warp

import (
	"errors"
	"strings"
	"time"
)

var (
	ErrNotFound     = errors.New("warp credentials not found")
	ErrInvalidInput = errors.New("invalid warp credentials input")
)

type Credentials struct {
	NodeID     string    `json:"node_id"`
	PrivateKey string    `json:"-"`
	PublicKey  string    `json:"public_key"`
	Address   string    `json:"address"`
	Endpoint  string    `json:"endpoint"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
}

type SetInput struct {
	NodeID     string
	PrivateKey string
	PublicKey  string
	Address   string
	Endpoint   string
}

func (i SetInput) Validate() error {
	if strings.TrimSpace(i.PrivateKey) == "" || strings.TrimSpace(i.PublicKey) == "" || strings.TrimSpace(i.Address) == "" {
		return ErrInvalidInput
	}
	return nil
}

type GenerateResult struct {
	PrivateKey string `json:"private_key"`
	PublicKey  string `json:"public_key"`
}
