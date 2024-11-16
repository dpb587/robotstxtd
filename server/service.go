package server

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"time"

	"github.com/dpb587/robotstxtd/api"
)

type Service struct {
	api.UnimplementedServiceServer

	log           *slog.Logger
	db            *sql.DB
	client        *http.Client
	clientTimeout time.Duration
	bot           *api.Bot

	getUpstreamTaskRequests chan getOriginUpstreamTaskRequest
}

var _ api.ServiceServer = &Service{}

func NewService(log *slog.Logger, db *sql.DB, client *http.Client, clientTimeout time.Duration, bot *api.Bot) *Service {
	if len(bot.Name) == 0 {
		panic("missing bot name")
	} else if len(bot.Version) == 0 {
		panic("missing bot version")
	} else if len(bot.Url) == 0 {
		panic("missing bot url")
	} else if len(bot.From) == 0 {
		panic("missing bot from")
	}

	s := &Service{
		log:                     log,
		db:                      db,
		client:                  client,
		clientTimeout:           clientTimeout,
		bot:                     bot,
		getUpstreamTaskRequests: make(chan getOriginUpstreamTaskRequest),
	}

	go s.getOriginUpstreamTaskRequestWorker()

	return s
}

func (s *Service) GetBot(ctx context.Context, req *api.GetBot_Request) (*api.GetBot_Response, error) {
	return &api.GetBot_Response{
		Bot: s.bot,
	}, nil
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
