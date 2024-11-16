package api

import (
	"fmt"
	"strconv"
	"time"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	statuspb "google.golang.org/genproto/googleapis/rpc/status"
	codes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (robotstxtEval *EvalUrl_Response) ToStatusProto() (*statuspb.Status, error) {
	switch rr := robotstxtEval.Result.(type) {
	case *EvalUrl_Response_ResultAllow_:
		return &statuspb.Status{}, nil
	case *EvalUrl_Response_ResultDisallow_:
		var errorInfoMetadata = map[string]string{
			"cache": robotstxtEval.LookupTime.AsTime().Format(time.RFC3339),
		}

		if rr.ResultDisallow.MatchLine != nil {
			errorInfoMetadata["matchPosition"] = "L" + strconv.FormatInt(int64(*rr.ResultDisallow.MatchLine), 10)
		}

		if rr.ResultDisallow.MatchLineContents != nil {
			errorInfoMetadata["matchContents"] = *rr.ResultDisallow.MatchLineContents
		}

		captureError := status.New(codes.Aborted, "robots.txt - Not Allowed")
		captureError, err := captureError.WithDetails(&errdetails.ErrorInfo{
			Reason:   "DISALLOW",
			Domain:   IRI,
			Metadata: errorInfoMetadata,
		})
		if err != nil {
			return nil, fmt.Errorf("build capture error: %v", err)
		}

		return captureError.Proto(), nil
	case *EvalUrl_Response_ResultError_:
		captureError := status.New(codes.Aborted, "robots.txt - Lookup Error")
		captureError, err := captureError.WithDetails(&errdetails.ErrorInfo{
			Reason: "ERROR",
			Domain: IRI,
			Metadata: map[string]string{
				"cache": robotstxtEval.LookupTime.AsTime().Format(time.RFC3339),
				"error": rr.ResultError.ErrorMessage,
			},
		})
		if err != nil {
			return nil, fmt.Errorf("build capture error: %v", err)
		}

		return captureError.Proto(), nil
	}

	return nil, fmt.Errorf("invalid result type: %T", robotstxtEval.Result)
}
