package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/dpb587/robotstxtd/api"
	"github.com/dpb587/robotstxtd/internal"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (s *Service) GetOrigin(ctx context.Context, req *api.GetOrigin_Request) (*api.GetOrigin_Response, error) {
	return s.getOrigin(req.Origin)
}

func (s *Service) getOrigin(origin string) (*api.GetOrigin_Response, error) {
	var (
		sLookupTime         string
		sResponseStatusCode uint32
		sResponseHeaders    sql.NullString
		sResponseBody       sql.NullString
		sResponseBodySha256 sql.NullString
		sErrorMessage       sql.NullString
	)

	err := s.db.QueryRow(
		`
			SELECT
				lookup_time,
				response_status_code,
				response_headers,
				response_body,
				response_body_sha256,
				error_message
			FROM cache
			WHERE origin = ? AND invalidated = 0 AND stale_time > ?
		`,
		origin,
		time.Now().Format(time.RFC3339),
	).Scan(
		&sLookupTime,
		&sResponseStatusCode,
		&sResponseHeaders,
		&sResponseBody,
		&sResponseBodySha256,
		&sErrorMessage,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return s.getOriginUpstream(origin)
		}

		return nil, err
	}

	lookupTime, err := time.Parse(time.RFC3339, sLookupTime)
	if err != nil {
		return nil, fmt.Errorf("lookup time: %v", err)
	}

	res := &api.GetOrigin_Response{
		LookupTime:         timestamppb.New(lookupTime),
		ResponseStatusCode: sResponseStatusCode,
	}

	if sResponseHeaders.Valid {
		res.ResponseHeaders = &sResponseHeaders.String
	}

	if sResponseBody.Valid {
		res.ResponseBody = &sResponseBody.String
	}

	if sResponseBodySha256.Valid {
		res.ResponseBodySha256 = &sResponseBodySha256.String
	}

	if sErrorMessage.Valid {
		res.ErrorMessage = &sErrorMessage.String
	}

	return res, nil
}

func (s *Service) getOriginUpstream(origin string) (*api.GetOrigin_Response, error) {
	if !internal.ValidateCanonicalOrigin(origin) {
		return nil, status.Error(codes.InvalidArgument, "invalid origin")
	}

	rc := make(chan getOriginUpstreamTaskResult, 1)

	s.getUpstreamTaskRequests <- getOriginUpstreamTaskRequest{
		origin: origin,
		result: rc,
	}

	result := <-rc

	return result.res, result.err
}

func (s *Service) getOriginUpstreamTaskRequestWorker() {
	s.log.Info("worker ready")
	defer s.log.Info("worker done")

	memo := make(map[string]*getOriginUpstreamTaskMemo)

	for req := range s.getUpstreamTaskRequests {
		e := memo[req.origin]
		if e == nil {
			e = &getOriginUpstreamTaskMemo{
				res:    getOriginUpstreamTaskResult{},
				waiter: make(chan struct{}),
			}

			memo[req.origin] = e

			go func() {
				e.res.res, e.res.err = s.getOriginUpstreamExecute(context.TODO(), req.origin)
				close(e.waiter)
			}()
		}

		go e.done(req.result)
	}
}

func (s *Service) getOriginUpstreamExecute(ctx context.Context, origin string) (*api.GetOrigin_Response, error) {
	s.log.Info(
		"fetch upstream",
		"origin", origin,
	)

	requestTime := time.Now()

	res, err := func() (*api.GetOrigin_Response, error) {
		dctx, cancel := context.WithDeadline(ctx, requestTime.Add(s.clientTimeout))
		defer cancel()

		req, err := http.NewRequestWithContext(
			dctx,
			http.MethodGet,
			fmt.Sprintf("%s/robots.txt", origin),
			nil,
		)
		if err != nil {
			return nil, err
		}

		req.Header.Set("User-Agent", s.bot.ClientString())

		res, err := s.client.Do(req)
		if err != nil {
			return nil, err
		}

		defer res.Body.Close()

		var resBodyString *string
		var resBodySha256Encoded *string

		if res.StatusCode >= 200 && res.StatusCode < 300 {
			const maxBodySize = 512000 // https://www.rfc-editor.org/rfc/rfc9309#section-2.5

			resBodySha256 := sha256.New()

			resBody, err := io.ReadAll(io.TeeReader(io.LimitReader(res.Body, maxBodySize+1), resBodySha256))
			if err != nil {
				return nil, err
			} else if len(resBody) > maxBodySize {
				s.log.Warn(
					"response was truncated",
					"origin", origin,
				)
			}

			_resBodyString := string(resBody)
			resBodyString = &_resBodyString

			_resBodySha256Encoded := base64.StdEncoding.EncodeToString(resBodySha256.Sum(nil))
			resBodySha256Encoded = &_resBodySha256Encoded
		}

		resHeadersBuffer := &bytes.Buffer{}
		err = res.Header.Write(resHeadersBuffer)
		if err != nil {
			return nil, err
		}

		resHeadersString := strings.TrimSpace(resHeadersBuffer.String())

		_, err = s.db.Exec(
			`
				UPDATE cache SET invalidated = 1 WHERE origin = ?;
				INSERT INTO cache (origin, invalidated, lookup_time, stale_time, response_status_code, response_headers, response_body, response_body_sha256) VALUES (?, 0, ?, ?, ?, ?, ?, ?);
			`,
			origin,
			origin,
			requestTime.Format(time.RFC3339),
			requestTime.Add(24*time.Hour).Format(time.RFC3339),
			uint32(res.StatusCode),
			resHeadersString,
			resBodyString,
			resBodySha256Encoded,
		)
		if err != nil {
			return nil, err
		}

		return &api.GetOrigin_Response{
			LookupTime:         timestamppb.New(requestTime),
			ResponseStatusCode: uint32(res.StatusCode),
			ResponseHeaders:    &resHeadersString,
			ResponseBody:       resBodyString,
			ResponseBodySha256: resBodySha256Encoded,
		}, nil
	}()
	if err != nil {
		errorMessage := err.Error()

		s.log.Error(
			"fetch failed",
			"origin", origin,
			"error", err,
		)

		_, err := s.db.Exec(
			`
				UPDATE cache SET invalidated = 1 WHERE origin = ?;
				INSERT INTO cache (origin, invalidated, lookup_time, stale_time, response_status_code, error_message) VALUES (?, 0, ?, ?, 0, ?);
			`,
			origin,
			origin,
			requestTime.Format(time.RFC3339),
			requestTime.Add(3*time.Hour).Format(time.RFC3339),
			errorMessage,
		)
		if err != nil {
			return nil, err
		}

		res = &api.GetOrigin_Response{
			LookupTime:         timestamppb.New(requestTime),
			ResponseStatusCode: 0,
			ErrorMessage:       &errorMessage,
		}
	}

	return res, nil
}

type getOriginUpstreamTaskRequest struct {
	origin string
	result chan<- getOriginUpstreamTaskResult
}

type getOriginUpstreamTaskResult struct {
	res *api.GetOrigin_Response
	err error
}

type getOriginUpstreamTaskMemo struct {
	res    getOriginUpstreamTaskResult
	waiter chan struct{}
}

func (m *getOriginUpstreamTaskMemo) done(out chan<- getOriginUpstreamTaskResult) {
	<-m.waiter
	out <- m.res
}
