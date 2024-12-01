package server

import (
	"context"
	"fmt"
	"net/url"

	"github.com/dpb587/robotstxtd/api"
	"github.com/dpb587/robotstxtd/api/apiutil"
	"github.com/dpb587/robotstxtd/internal"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Service) EvalUrl(ctx context.Context, req *api.EvalUrl_Request) (*api.EvalUrl_Response, error) {
	reqURL, err := url.Parse(req.Url)
	if err != nil {
		s.log.Info(
			"sending error to client",
			"method", "EvalUrl",
			"error", err,
		)

		return nil, status.Error(codes.InvalidArgument, "url could not be parsed")
	}

	res, err := s.getOrigin(internal.Origin(reqURL))
	if err != nil {
		err = fmt.Errorf("get origin: %v", err)

		s.log.Info(
			"sending error to client",
			"method", "EvalUrl",
			"error", err,
		)

		return nil, err
	}

	return apiutil.Evaluate(res, s.bot, req.Url, true), nil
}
