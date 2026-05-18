package storage

import (
	"context"
	"database/sql"
	"errors"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/lenker/lenker/services/panel-api/internal/admins"
	"github.com/lenker/lenker/services/panel-api/internal/configrender"
)

var ErrNotFound = errors.New("storage resource not found")

type Config struct {
	DatabaseURL string
	Ping        bool
	Reality     configrender.RealityConfig
}

type Store struct {
	db            *sql.DB
	admins        admins.Repository
	users         UsersRepository
	plans         PlansRepository
	subscriptions SubscriptionsRepository
	nodes         NodesRepository
}

func Open(ctx context.Context, cfg Config) (*Store, error) {
	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}

	if cfg.Ping {
		if err := db.PingContext(ctx); err != nil {
			_ = db.Close()
			return nil, err
		}
	}

	return &Store{
		db:            db,
		admins:        NewAdminsRepository(db),
		users:         NewUsersRepository(db),
		plans:         NewPlansRepository(db),
		subscriptions: NewSubscriptionsRepositoryWithReality(db, cfg.Reality),
		nodes:         NewNodesRepositoryWithReality(db, cfg.Reality),
	}, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) DB() *sql.DB {
	return s.db
}

func (s *Store) Admins() admins.Repository {
	return s.admins
}

func (s *Store) Users() UsersRepository {
	return s.users
}

func (s *Store) Plans() PlansRepository {
	return s.plans
}

func (s *Store) Subscriptions() SubscriptionsRepository {
	return s.subscriptions
}

func (s *Store) Nodes() NodesRepository {
	return s.nodes
}
