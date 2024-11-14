package server

import (
	"context"
	"fmt"
	"time"

	"github.com/dpb587/robotstxtd/api"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (s *Service) FlushOrigin(ctx context.Context, req *api.FlushOrigin_Request) (*api.FlushOrigin_Response, error) {
	s.log.Info(
		"flush origin",
		"origin", req.Origin,
	)

	lookupTime := req.MinLookupTime
	if lookupTime == nil {
		lookupTime = timestamppb.New(time.Now().Add(5 * time.Second))
	}

	_, err := s.db.Exec(
		`
			UPDATE cache
			SET invalidated = 1
			WHERE origin = ? AND lookup_time < ?;
		`,
		req.Origin,
		lookupTime.AsTime().Format(time.RFC3339),
	)
	if err != nil {
		return nil, fmt.Errorf("db: %v", err)
	}

	return &api.FlushOrigin_Response{}, nil
}
