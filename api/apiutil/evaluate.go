package apiutil

import (
	"strings"

	"github.com/dpb587/robotstxtd/api"
	"github.com/jimsmart/grobotstxt"
)

func Evaluate(r *api.GetOrigin_Response, bot *api.Bot, url string, verbose bool) *api.EvalUrl_Response {
	result := &api.EvalUrl_Response{
		LookupTime: r.LookupTime,
	}

	if r.ErrorMessage != nil {
		result.Result = &api.EvalUrl_Response_ResultError_{
			ResultError: &api.EvalUrl_Response_ResultError{
				ErrorMessage: *r.ErrorMessage,
			},
		}

		return result
	} else if r.ResponseStatusCode >= 400 && r.ResponseStatusCode < 500 {
		result.Result = &api.EvalUrl_Response_ResultAllow_{
			ResultAllow: &api.EvalUrl_Response_ResultAllow{},
		}

		return result
	} else if r.ResponseStatusCode >= 200 && r.ResponseStatusCode < 300 {
		// continue
	} else {
		result.Result = &api.EvalUrl_Response_ResultDisallow_{
			ResultDisallow: &api.EvalUrl_Response_ResultDisallow{},
		}

		return result
	}

	if r.ResponseBody == nil {
		result.Result = &api.EvalUrl_Response_ResultAllow_{
			ResultAllow: &api.EvalUrl_Response_ResultAllow{},
		}

		return result
	}

	matcher := grobotstxt.NewRobotsMatcher()
	allowed := matcher.AgentAllowed(*r.ResponseBody, bot.Name, url)

	if allowed {
		result.Result = &api.EvalUrl_Response_ResultAllow_{
			ResultAllow: &api.EvalUrl_Response_ResultAllow{},
		}

		return result
	}

	resultDisallow := &api.EvalUrl_Response_ResultDisallow{}

	if verbose {
		if matchingLine := uint32(matcher.MatchingLine()); matchingLine > 0 {
			resultDisallow.MatchLine = &matchingLine

			bodyLines := strings.Split(*r.ResponseBody, "\n")

			if uint32(len(bodyLines)) > matchingLine {
				matchLineContents := bodyLines[matchingLine-1]
				resultDisallow.MatchLineContents = &matchLineContents
			}
		}
	}

	result.Result = &api.EvalUrl_Response_ResultDisallow_{
		ResultDisallow: resultDisallow,
	}

	return result
}
