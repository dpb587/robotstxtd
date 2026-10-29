package server

import (
	"context"
	"fmt"
	"time"

	"github.com/dpb587/robotstxtd/robotstxtdpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (s *Service) InvalidateFileCache(ctx context.Context, req *robotstxtdpb.Crawler_InvalidateFileCache_Request) (*robotstxtdpb.Crawler_InvalidateFileCache_Response, error) {
	lookupTime := req.BeforeTime
	if lookupTime == nil {
		lookupTime = timestamppb.New(time.Now().Add(5 * time.Second))
	}

	res, err := s.db.Exec(
		`
			UPDATE cache
			SET invalidate_time = ?
			WHERE
				service_uri = ?
				AND request_time < ?
				AND invalidate_time IS NULL
		`,
		time.Now().Format(time.RFC3339),
		req.ServiceUri,
		lookupTime.AsTime().Format(time.RFC3339),
	)
	if err != nil {
		return nil, fmt.Errorf("db: %v", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("db: %v", err)
	}

	s.log.Info(
		"invalidate file cache",
		"service", req.ServiceUri,
		"count", affected,
	)

	return &robotstxtdpb.Crawler_InvalidateFileCache_Response{}, nil
}
