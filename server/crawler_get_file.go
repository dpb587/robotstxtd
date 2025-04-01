package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/dpb587/robotstxtd/internal"
	"github.com/dpb587/robotstxtd/robotstxtdpb"
	"github.com/google/uuid"
	statuspb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (s *Service) GetFile(ctx context.Context, req *robotstxtdpb.Crawler_GetFile_Request) (*robotstxtdpb.File, error) {
	return s.getFile(req.ServiceUri)
}

func (s *Service) getFile(serviceUri string) (*robotstxtdpb.File, error) {
	var (
		sCacheKey              string
		sRequestTime           string
		sErrorCode             sql.NullInt32
		sErrorMessage          sql.NullString
		sResponseStatusCode    uint32
		sResponseHeaders       sql.NullString
		sResponseBodyText      sql.NullString
		sResponseBodyTruncated sql.NullInt32
	)

	err := s.db.QueryRow(
		`
			SELECT
				cache_key,
				request_time,
				error_code,
				error_message,
				response_status_code,
				response_headers,
				response_body_text,
				response_body_truncated
			FROM cache2
			WHERE service_uri = ? AND invalidate_time IS NULL AND stale_time > ?
		`,
		serviceUri,
		time.Now().Format(time.RFC3339),
	).Scan(
		&sCacheKey,
		&sRequestTime,
		&sErrorCode,
		&sErrorMessage,
		&sResponseStatusCode,
		&sResponseHeaders,
		&sResponseBodyText,
		&sResponseBodyTruncated,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return s.getFileUpstream(serviceUri)
		}

		return nil, err
	}

	requestTime, err := time.Parse(time.RFC3339, sRequestTime)
	if err != nil {
		return nil, fmt.Errorf("lookup time: %v", err)
	}

	res := &robotstxtdpb.File{
		Authority: &robotstxtdpb.File_Authority{
			ServiceUri: serviceUri,
			CacheKey:   sCacheKey,
		},
		Upstream: &robotstxtdpb.File_Upstream{
			Uri:  fmt.Sprintf("%s/robots.txt", serviceUri),
			Time: timestamppb.New(requestTime),
		},
	}

	if sErrorCode.Valid {
		res.Upstream.Result = &robotstxtdpb.File_Upstream_Error{
			Error: &statuspb.Status{
				Code:    int32(sErrorCode.Int32),
				Message: sErrorMessage.String,
			},
		}
	} else {
		res.Upstream.Result = &robotstxtdpb.File_Upstream_Response_{
			Response: &robotstxtdpb.File_Upstream_Response{
				StatusCode: sResponseStatusCode,
				Headers:    sResponseHeaders.String,
			},
		}

		if sResponseBodyText.Valid {
			res.Body = &robotstxtdpb.File_Body{
				Text:      sResponseBodyText.String,
				Truncated: sResponseBodyTruncated.Int32 > 0,
			}
		} else {
			res.Body = &robotstxtdpb.File_Body{}
		}
	}

	return res, nil
}

func (s *Service) getFileUpstream(serviceUri string) (*robotstxtdpb.File, error) {
	if !internal.ValidateCanonicalOrigin(serviceUri) {
		return nil, status.Error(codes.InvalidArgument, "invalid service uri")
	}

	rc := make(chan queueGetFileUpstreamResult, 1)

	s.getFileUpstreamTaskRequests <- queueGetFileUpstreamRequest{
		serviceUri: serviceUri,
		result:     rc,
	}

	result := <-rc

	return result.res, result.err
}

func (s *Service) queueGetFileUpstreamWorker() {
	memo := make(map[string]*queueGetFileUpstreamExecution)
	prune := time.NewTicker(time.Minute)

	for {
		select {
		case req, ok := <-s.getFileUpstreamTaskRequests:
			if !ok {
				return
			}

			e := memo[req.serviceUri]
			if e == nil {
				e = &queueGetFileUpstreamExecution{
					res:    queueGetFileUpstreamResult{},
					waiter: make(chan struct{}),
				}

				memo[req.serviceUri] = e

				go func() {
					e.res.res, e.res.err = s.queueGetFileUpstreamExecute(context.TODO(), req.serviceUri)
					close(e.waiter)
				}()
			}

			go e.wait(req.result)
		case <-prune.C:
			for k, e := range memo {
				if e.waiter == nil {
					delete(memo, k)
				}
			}
		}
	}
}

func (s *Service) queueGetFileUpstreamExecute(ctx context.Context, serviceUri string) (*robotstxtdpb.File, error) {
	s.log.Info(
		"fetch upstream",
		"service", serviceUri,
	)

	requestTime := time.Now()
	cacheKeyUUID, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("generate cache key: %v", err)
	}

	cacheKey := cacheKeyUUID.String()

	res, errStatus := func() (*robotstxtdpb.File, *statuspb.Status) {
		dctx, cancel := context.WithDeadline(ctx, requestTime.Add(s.clientTimeout))
		defer cancel()

		req, err := http.NewRequestWithContext(
			dctx,
			http.MethodGet,
			fmt.Sprintf("%s/robots.txt", serviceUri),
			nil,
		)
		if err != nil {
			return nil, &statuspb.Status{
				Code:    int32(codes.InvalidArgument),
				Message: "invalid argument",
			}
		}

		req.Header.Set("User-Agent", s.bot.ClientString())

		res, err := s.client.Do(req)
		if err != nil {
			return nil, &statuspb.Status{
				Code:    int32(codes.Unavailable),
				Message: "service unavailable",
			}
		}

		defer res.Body.Close()

		resHeadersBuffer := &bytes.Buffer{}
		err = res.Header.Write(resHeadersBuffer)
		if err != nil {
			return nil, &statuspb.Status{
				Code:    int32(codes.Internal),
				Message: "internal error",
			}
		}

		var resBodyText sql.NullString
		var resBodyTruncated sql.NullInt32

		if res.StatusCode >= 200 && res.StatusCode < 300 {
			const maxBodySize = 512000 // https://www.rfc-editor.org/rfc/rfc9309#section-2.5

			resBodySha256 := sha256.New()

			resBody, err := io.ReadAll(io.TeeReader(io.LimitReader(res.Body, maxBodySize+1), resBodySha256))
			if err != nil {
				return nil, &statuspb.Status{
					Code:    int32(codes.Unavailable),
					Message: "service unavailable",
				}
			}

			if len(resBody) > maxBodySize {
				resBodyText.Valid, resBodyTruncated.Valid = true, true
				resBodyText.String, resBodyTruncated.Int32 = resBodyText.String[:maxBodySize], 1
			} else if len(resBody) > 0 {
				resBodyText.Valid, resBodyTruncated.Valid = true, true
				resBodyText.String, resBodyTruncated.Int32 = string(resBody), 0
			}
		}

		resProto := &robotstxtdpb.File{
			Authority: &robotstxtdpb.File_Authority{
				ServiceUri: serviceUri,
				CacheKey:   cacheKey,
			},
			Upstream: &robotstxtdpb.File_Upstream{
				Uri:  fmt.Sprintf("%s/robots.txt", serviceUri),
				Time: timestamppb.New(requestTime),
				Result: &robotstxtdpb.File_Upstream_Response_{
					Response: &robotstxtdpb.File_Upstream_Response{
						StatusCode: uint32(res.StatusCode),
						Headers:    resHeadersBuffer.String(),
					},
				},
			},
			Body: &robotstxtdpb.File_Body{
				Text:      resBodyText.String,
				Truncated: resBodyTruncated.Int32 > 0,
			},
		}

		_, err = s.db.Exec(
			`
				UPDATE cache2 SET invalidate_time = ? WHERE service_uri = ?;
				INSERT INTO cache2 (service_uri, cache_key, request_time, stale_time, response_status_code, response_headers, response_body_text, response_body_truncated) VALUES (?, ?, ?, ?, ?, ?, ?, ?);
			`,
			// update
			time.Now().Format(time.RFC3339),
			serviceUri,
			// insert
			serviceUri,
			cacheKey,
			requestTime.Format(time.RFC3339),
			requestTime.Add(24*time.Hour).Format(time.RFC3339),
			uint32(res.StatusCode),
			strings.TrimSpace(resHeadersBuffer.String()),
			resBodyText,
			resBodyTruncated,
		)
		if err != nil {
			return nil, &statuspb.Status{
				Code:    int32(codes.Internal),
				Message: "internal error",
			}
		}

		return resProto, nil
	}()
	if errStatus != nil {
		s.log.Error(
			"fetch failed",
			"service", serviceUri,
			"error", errStatus.Message,
		)

		_, err := s.db.Exec(
			`
				UPDATE cache2 SET invalidate_time = ? WHERE service_uri = ?;
				INSERT INTO cache2 (service_uri, cache_key, request_time, stale_time, error_code, error_message) VALUES (?, ?, ?, ?, ?, ?);
			`,
			// update
			serviceUri,
			time.Now().Format(time.RFC3339),
			// insert
			serviceUri,
			cacheKey,
			requestTime.Format(time.RFC3339),
			requestTime.Add(3*time.Hour).Format(time.RFC3339),
			errStatus.Code,
			errStatus.Message,
		)
		if err != nil {
			return nil, err
		}

		res = &robotstxtdpb.File{
			Authority: &robotstxtdpb.File_Authority{
				ServiceUri: serviceUri,
				CacheKey:   cacheKey,
			},
			Upstream: &robotstxtdpb.File_Upstream{
				Uri:  fmt.Sprintf("%s/robots.txt", serviceUri),
				Time: timestamppb.New(requestTime),
				Result: &robotstxtdpb.File_Upstream_Error{
					Error: errStatus,
				},
			},
		}
	}

	return res, nil
}

type queueGetFileUpstreamRequest struct {
	serviceUri string
	result     chan<- queueGetFileUpstreamResult
}

type queueGetFileUpstreamResult struct {
	res *robotstxtdpb.File
	err error
}

type queueGetFileUpstreamExecution struct {
	res    queueGetFileUpstreamResult
	waiter chan struct{}
}

func (m *queueGetFileUpstreamExecution) wait(out chan<- queueGetFileUpstreamResult) {
	<-m.waiter
	out <- m.res
}
