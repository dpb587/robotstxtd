package server

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"time"

	"github.com/dpb587/robotstxtd/robotstxtdpb"
	"golang.org/x/sync/singleflight"
)

type Service struct {
	robotstxtdpb.UnimplementedCrawlerServer

	log           *slog.Logger
	db            *sql.DB
	client        *http.Client
	clientTimeout time.Duration
	bot           *robotstxtdpb.UserAgent

	getFileGroup singleflight.Group
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

	return &Service{
		log:           log,
		db:            db,
		client:        client,
		clientTimeout: clientTimeout,
		bot:           bot,
	}
}

func (s *Service) GetBot(ctx context.Context, req *robotstxtdpb.Crawler_GetBot_Request) (*robotstxtdpb.UserAgent, error) {
	return s.bot, nil
}

func (s *Service) Prune() error {
	res, err := s.db.Exec(
		`
			DELETE FROM cache
			WHERE invalidate_time IS NOT NULL
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

	return nil
}

func (s *Service) Close() error {
	return nil
}
