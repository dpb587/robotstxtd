package server

import (
	"context"
	"fmt"
	"net/url"

	"github.com/dpb587/robotstxtd/agent"
	"github.com/dpb587/robotstxtd/api"
	"github.com/dpb587/robotstxtd/internal"
)

func (s *Service) Eval(ctx context.Context, req *api.Eval_Request) (*api.Eval_Response, error) {
	reqURL, err := url.Parse(req.Url)
	if err != nil {
		return nil, fmt.Errorf("invalid url: %v", err)
	}

	res, err := s.get(internal.Origin(reqURL))
	if err != nil {
		return nil, fmt.Errorf("get: %v", err)
	}

	result := &api.Eval_Response{
		LookupTime: res.LookupTime,
	}

	if res.ErrorMessage != nil {
		result.Result = &api.Eval_Response_ResultError_{
			ResultError: &api.Eval_Response_ResultError{
				ErrorMessage: *res.ErrorMessage,
			},
		}

		return result, nil
	} else if res.ResponseStatusCode >= 400 && res.ResponseStatusCode < 500 {
		result.Result = &api.Eval_Response_ResultAllow_{
			ResultAllow: &api.Eval_Response_ResultAllow{},
		}

		return result, nil
	} else if res.ResponseStatusCode >= 200 && res.ResponseStatusCode < 300 {
		// continue
	} else {
		result.Result = &api.Eval_Response_ResultDisallow_{
			ResultDisallow: &api.Eval_Response_ResultDisallow{},
		}

		return result, nil
	}

	if res.ResponseBody == nil {
		result.Result = &api.Eval_Response_ResultAllow_{
			ResultAllow: &api.Eval_Response_ResultAllow{},
		}

		return result, nil
	}

	result.Result = agent.Evaluate(s.bot.Name, *res.ResponseBody, req.Url, true)

	return result, nil
}
