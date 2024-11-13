package agent

import (
	"strings"

	"github.com/dpb587/robotstxtd/api"
	"github.com/jimsmart/grobotstxt"
)

func Evaluate(botName, contents, url string, verbose bool) api.Eval_Response_Result {
	matcher := grobotstxt.NewRobotsMatcher()
	allowed := matcher.AgentAllowed(contents, botName, url)

	if allowed {
		return &api.Eval_Response_ResultAllow_{
			ResultAllow: &api.Eval_Response_ResultAllow{},
		}
	}

	resultDisallow := &api.Eval_Response_ResultDisallow{}

	if verbose {
		if matchingLine := uint32(matcher.MatchingLine()); matchingLine > 0 {
			resultDisallow.MatchLine = &matchingLine

			bodyLines := strings.Split(contents, "\n")

			if uint32(len(bodyLines)) > matchingLine {
				matchLineContents := bodyLines[matchingLine-1]
				resultDisallow.MatchLineContents = &matchLineContents
			}
		}
	}

	return &api.Eval_Response_ResultDisallow_{
		ResultDisallow: resultDisallow,
	}
}
