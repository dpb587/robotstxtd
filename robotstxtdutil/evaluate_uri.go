package robotstxtdutil

import (
	"fmt"
	"net/http"

	"github.com/dpb587/robotstxtd/robotstxtdpb"
	"github.com/jimsmart/grobotstxt"
)

func EvaluateUri(r *robotstxtdpb.File, userAgentIdentifier, url string) *robotstxtdpb.UriEvaluation {
	result := &robotstxtdpb.UriEvaluation{
		Authority: r.Authority,
	}

	switch upstreamResult := r.Upstream.Result.(type) {
	case *robotstxtdpb.File_Upstream_Error:
		result.Access = &robotstxtdpb.UriEvaluation_DisallowAccess{
			DisallowAccess: &robotstxtdpb.UriEvaluation_Access_Disallow{
				Reason: &robotstxtdpb.UriEvaluation_Access_Disallow_UnreachableStatus{
					UnreachableStatus: true,
				},
			},
		}

		return result
	case *robotstxtdpb.File_Upstream_Response_:
		if upstreamResult.Response.StatusCode >= 400 && upstreamResult.Response.StatusCode < 500 {
			if upstreamResult.Response.StatusCode == http.StatusTooManyRequests {
				result.Access = &robotstxtdpb.UriEvaluation_DisallowAccess{
					DisallowAccess: &robotstxtdpb.UriEvaluation_Access_Disallow{
						Reason: &robotstxtdpb.UriEvaluation_Access_Disallow_UnreachableStatus{
							UnreachableStatus: true,
						},
					},
				}
			} else {
				result.Access = &robotstxtdpb.UriEvaluation_AllowAccess{
					AllowAccess: &robotstxtdpb.UriEvaluation_Access_Allow{
						Reason: &robotstxtdpb.UriEvaluation_Access_Allow_UnavailableStatus{
							UnavailableStatus: true,
						},
					},
				}
			}

			return result
		} else if upstreamResult.Response.StatusCode >= 200 && upstreamResult.Response.StatusCode < 300 {
			// continue
		} else {
			result.Access = &robotstxtdpb.UriEvaluation_DisallowAccess{
				DisallowAccess: &robotstxtdpb.UriEvaluation_Access_Disallow{
					Reason: &robotstxtdpb.UriEvaluation_Access_Disallow_UnreachableStatus{
						UnreachableStatus: true,
					},
				},
			}

			return result
		}

		matcher := grobotstxt.NewRobotsMatcher()

		if matcher.AgentAllowed(r.Body.Text, userAgentIdentifier, url) {
			if matchingLine := matcher.MatchingLine(); matchingLine > 0 {
				result.Access = &robotstxtdpb.UriEvaluation_AllowAccess{
					AllowAccess: &robotstxtdpb.UriEvaluation_Access_Allow{
						Reason: &robotstxtdpb.UriEvaluation_Access_Allow_RuleMatchLine{
							RuleMatchLine: uint32(matchingLine),
						},
					},
				}
			} else {
				result.Access = &robotstxtdpb.UriEvaluation_AllowAccess{
					AllowAccess: &robotstxtdpb.UriEvaluation_Access_Allow{
						Reason: &robotstxtdpb.UriEvaluation_Access_Allow_NoRuleMatch{
							NoRuleMatch: true,
						},
					},
				}
			}

			return result
		}

		result.Access = &robotstxtdpb.UriEvaluation_DisallowAccess{
			DisallowAccess: &robotstxtdpb.UriEvaluation_Access_Disallow{
				Reason: &robotstxtdpb.UriEvaluation_Access_Disallow_RuleMatchLine{
					RuleMatchLine: uint32(matcher.MatchingLine()),
				},
			},
		}

		return result
	}

	panic(fmt.Errorf("unexpected result type: %T", r.Upstream.Result))
}
