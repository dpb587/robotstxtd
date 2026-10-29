package robotstxtdpb

import (
	"fmt"
	"strconv"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	codes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var statusOK = status.New(codes.OK, "OK")

func (robotstxtEval *UriEvaluation) ToStatusProto() (*status.Status, error) {
	switch rr := robotstxtEval.Access.(type) {
	case *UriEvaluation_AllowAccess:
		return statusOK, nil
	case *UriEvaluation_DisallowAccess:
		errorInfo := &errdetails.ErrorInfo{
			Domain: "github.com/dpb587/robotstxtd",
			Metadata: map[string]string{
				"authorityServiceUri": robotstxtEval.Authority.ServiceUri,
				"authorityGeneration": robotstxtEval.Authority.Generation,
			},
		}

		switch reason := rr.DisallowAccess.Reason.(type) {
		case *UriEvaluation_Access_Disallow_UnreachableStatus:
			errorInfo.Reason = "STATUS_UNREACHABLE"
		case *UriEvaluation_Access_Disallow_RuleMatchLine:
			errorInfo.Reason = "RULE_DISALLOW"
			errorInfo.Metadata["contentsOffset"] = "L" + strconv.FormatInt(int64(reason.RuleMatchLine), 10)
		default:
			panic(fmt.Errorf("robotstxtdpb: unexpected disallow reason: %T", reason))
		}

		return status.New(codes.FailedPrecondition, "robots.txt - Not Allowed").WithDetails(errorInfo)
	}

	return nil, fmt.Errorf("robotstxtdpb: unexpected result type: %T", robotstxtEval.Access)
}
