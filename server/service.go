package server

import (
	"database/sql"
	"log/slog"
	"net/http"
	"time"

	"github.com/dpb587/robotstxtd/agent"
	"github.com/dpb587/robotstxtd/api"
)

type Service struct {
	api.UnimplementedServiceServer

	log           *slog.Logger
	db            *sql.DB
	client        *http.Client
	clientTimeout time.Duration
	bot           agent.Bot

	getUpstreamTaskRequests chan getUpstreamTaskRequest
}

var _ api.ServiceServer = &Service{}

func NewService(log *slog.Logger, db *sql.DB, client *http.Client, clientTimeout time.Duration, bot agent.Bot) *Service {
	if len(bot.Name) == 0 {
		panic("missing bot name")
	} else if len(bot.Version) == 0 {
		panic("missing bot version")
	} else if len(bot.URL) == 0 {
		panic("missing bot url")
	}

	s := &Service{
		log:                     log,
		db:                      db,
		client:                  client,
		clientTimeout:           clientTimeout,
		bot:                     bot,
		getUpstreamTaskRequests: make(chan getUpstreamTaskRequest),
	}

	go s.getUpstreamTaskRequestWorker()

	return s
}

func (s *Service) Prune() error {
	res, err := s.db.Exec(
		`
			DELETE FROM cache
			WHERE invalidated = 1
		`,
	)
	if err != nil {
		return err
	}

	resCount, err := res.RowsAffected()
	if err != nil {
		return err
	}

	s.log.Info(
		"pruned database",
		"rows", resCount,
	)

	return err
}

func (s *Service) Close() error {
	close(s.getUpstreamTaskRequests)

	return nil
}
