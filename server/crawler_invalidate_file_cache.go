package server

import (
	"context"
	"fmt"
	"time"

	"github.com/dpb587/robotstxtd/robotstxtdpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (s *Service) InvalidateFileCache(ctx context.Context, req *robotstxtdpb.Crawler_InvalidateFileCache_Request) (*robotstxtdpb.Crawler_InvalidateFileCache_Response, error) {
	s.log.Info(
		"invalidate file cache",
		"service", req.ServiceUri,
	)

	lookupTime := req.BeforeTime
	if lookupTime == nil {
		lookupTime = timestamppb.New(time.Now().Add(5 * time.Second))
	}

	_, err := s.db.Exec(
		`
			UPDATE cache
			SET invalidated = 1
			WHERE origin = ? AND lookup_time < ?;
		`,
		req.ServiceUri,
		lookupTime.AsTime().Format(time.RFC3339),
	)
	if err != nil {
		return nil, fmt.Errorf("db: %v", err)
	}

	return &robotstxtdpb.Crawler_InvalidateFileCache_Response{}, nil
}
