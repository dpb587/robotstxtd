package server

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"time"

	"github.com/dpb587/robotstxtd/robotstxtdpb"
)

type Service struct {
	robotstxtdpb.UnimplementedCrawlerServer

	log           *slog.Logger
	db            *sql.DB
	client        *http.Client
	clientTimeout time.Duration
	bot           *robotstxtdpb.UserAgent

	getFileUpstreamTaskRequests chan queueGetFileUpstreamRequest
}

var _ robotstxtdpb.CrawlerServer = &Service{}

func NewService(log *slog.Logger, db *sql.DB, client *http.Client, clientTimeout time.Duration, bot *robotstxtdpb.UserAgent) *Service {
	if len(bot.Identifier) == 0 {
		panic("missing user agent identifier")
	} else if len(bot.Version) == 0 {
		panic("missing user agent version")
	} else if len(bot.Url) == 0 {
		panic("missing user agent url")
	} else if len(bot.Email) == 0 {
		panic("missing user agent email")
	}

	s := &Service{
		log:                         log,
		db:                          db,
		client:                      client,
		clientTimeout:               clientTimeout,
		bot:                         bot,
		getFileUpstreamTaskRequests: make(chan queueGetFileUpstreamRequest, 128),
	}

	go func() {
		s.log.Info("thread (GetFileUpstream) started")
		defer s.log.Info("thread (GetFileUpstream) done")

		s.queueGetFileUpstreamWorker()
	}()

	return s
}

func (s *Service) GetBot(ctx context.Context, req *robotstxtdpb.Crawler_GetBot_Request) (*robotstxtdpb.UserAgent, error) {
	return s.bot, nil
}

func (s *Service) Prune() error {
	res, err := s.db.Exec(
		`
			DELETE FROM cache2
			WHERE invalidate_time IS NULL
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
	close(s.getFileUpstreamTaskRequests)

	return nil
}
