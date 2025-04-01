package server

import (
	"context"
	"fmt"
	"net/url"

	"github.com/dpb587/robotstxtd/internal"
	"github.com/dpb587/robotstxtd/robotstxtdpb"
	"github.com/dpb587/robotstxtd/robotstxtdutil"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Service) EvaluateUri(ctx context.Context, req *robotstxtdpb.Crawler_EvaluateUri_Request) (*robotstxtdpb.UriEvaluation, error) {
	reqURL, err := url.Parse(req.Uri)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("parse uri: %v", err))
	}

	res, err := s.getFile(internal.Origin(reqURL))
	if err != nil {
		return nil, err
	}

	return robotstxtdutil.EvaluateUri(res, s.bot.Identifier, req.Uri), nil
}
