package storage

import (
	"context"
	"embed"
	"errors"
	"time"

	"github.com/callmedenchick/callmebridge/internal/models"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v4/pgxpool"
	log "github.com/sirupsen/logrus"
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

func (s *PgStorage) Add(ctx context.Context, key string, ttl int64, mes models.SseMessage) error {
	_, err := s.postgres.Exec(ctx, `
		INSERT INTO bridge.messages
		(
		client_id,
		event_id,
		end_time,
		bridge_message
		)
		VALUES ($1, $2, to_timestamp($3), $4)
	`, key, mes.EventId, time.Now().Add(time.Duration(ttl)*time.Second).Unix(), mes.Message)
	if err != nil {
		return err
	}
	return nil
}

func (s *PgStorage) GetMessages(ctx context.Context, keys []string, lastEventId int64) ([]models.SseMessage, error) { // interface{}
	log := log.WithField("prefix", "Storage.GetQueue")
	var messages []models.SseMessage
	rows, err := s.postgres.Query(ctx, `SELECT event_id, bridge_message
	FROM bridge.messages
	WHERE current_timestamp < end_time 
	AND event_id > $1
	AND client_id = any($2)`, lastEventId, keys)
	if err != nil {
		log.Info(err)
		return nil, err
	}
	for rows.Next() {
		var mes models.SseMessage
		err = rows.Scan(&mes.EventId, &mes.Message)
		if err != nil {
			log.Info(err)
			return nil, err
		}
		messages = append(messages, mes)
	}
	return messages, nil
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
