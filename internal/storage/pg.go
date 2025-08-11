package storage

import (
	"context"
	"embed"
	"errors"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v4/pgxpool"
	log "github.com/sirupsen/logrus"
	"github.com/ton-connect/bridge3/internal/models"
)

type Message []byte
type PgStorage struct {
	postgres *pgxpool.Pool
}

//go:embed migrations/*.sql
var fs embed.FS

func MigrateDb(postgresURI string) error {
	log := log.WithField("prefix", "MigrateDb")
	d, err := iofs.New(fs, "migrations")
	if err != nil {
		log.Info("iofs err: ", err)
		return err
	}
	m, err := migrate.NewWithSourceInstance("iofs", d, postgresURI)
	if err != nil {
		log.Info("source instance err: ", err)
		return err
	}
	err = m.Up()
	if errors.Is(err, migrate.ErrNoChange) {
		log.Info("DB is up to date")
		return nil
	} else if err != nil {
		return err
	}
	log.Info("DB updated successfully")
	return nil
}

func NewPgStorage(postgresURI string) (*PgStorage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	log := log.WithField("prefix", "NewStorage")
	defer cancel()
	c, err := pgxpool.Connect(ctx, postgresURI)
	if err != nil {
		return nil, err
	}
	err = MigrateDb(postgresURI)
	if err != nil {
		log.Info("migrte err: ", err)
		return nil, err
	}
	s := PgStorage{
		postgres: c,
	}
	go s.worker()
	return &s, nil
}

func (s *PgStorage) worker() {
	log := log.WithField("prefix", "Storage.worker")
	for {
		<-time.NewTimer(time.Minute).C
		log.Info("time to db check")
		_, err := s.postgres.Exec(context.TODO(),
			`DELETE FROM bridge.messages 
			 	 WHERE current_timestamp > end_time`)
		if err != nil {
			log.Infof("remove expired messages error: %v", err)
		}
	}

}

// Pub is not implemented for Postgres storage
func (s *PgStorage) Pub(ctx context.Context, key string, ttl int64, message models.SseMessage) error {
	return errors.New("pub-sub not implemented for Postgres storage")
}

// Sub is not implemented for Postgres storage
func (s *PgStorage) Sub(ctx context.Context, keys []string, lastEventId int64, messageCh chan<- models.SseMessage) error {
	return errors.New("pub-sub not implemented for Postgres storage")
}

// Unsub is not implemented for Postgres storage
func (s *PgStorage) Unsub(ctx context.Context, keys []string) error {
	return errors.New("pub-sub not implemented for Postgres storage")
}

func (s *PgStorage) HealthCheck() error {
	log := log.WithField("prefix", "Storage.HealthCheck")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	err := s.postgres.Ping(ctx)
	if err != nil {
		log.Errorf("database health check failed: %v", err)
		return err
	}
	log.Info("database is healthy")
	return nil
}
