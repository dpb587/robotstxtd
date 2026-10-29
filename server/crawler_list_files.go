package server

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/dpb587/robotstxtd/robotstxtdpb"
	statuspb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	defaultListFilesPageSize uint32 = 50
	maxListFilesPageSize     uint32 = 200
)

func (s *Service) ListFiles(ctx context.Context, req *robotstxtdpb.Crawler_ListFiles_Request) (*robotstxtdpb.Crawler_ListFiles_Response, error) {
	pageSize := req.GetPageSize()
	if pageSize == 0 {
		pageSize = defaultListFilesPageSize
	}
	if pageSize > maxListFilesPageSize {
		pageSize = maxListFilesPageSize
	}

	includeAll := 0
	if req.GetIncludeInvalidated() {
		includeAll = 1
	}

	rows, err := s.db.QueryContext(
		ctx,
		`
			SELECT
				service_uri,
				generation,
				request_time,
				stale_time,
				error_code,
				error_message,
				response_status_code,
				invalidate_time
			FROM cache
			WHERE
				(? = '' OR generation < ?)
				AND (? = 1 OR invalidate_time IS NULL)
			ORDER BY generation DESC
			LIMIT ?
		`,
		req.GetPageToken(),
		req.GetPageToken(),
		includeAll,
		pageSize+1,
	)
	if err != nil {
		return nil, fmt.Errorf("db: %v", err)
	}
	defer rows.Close()

	files := make([]*robotstxtdpb.Crawler_ListFiles_Summary, 0, pageSize)
	for rows.Next() {
		var (
			serviceURI     string
			generation     string
			requestTimeRaw string
			staleTimeRaw   string
			errorCode      sql.NullInt32
			errorMessage   sql.NullString
			statusCode     sql.NullInt32
			invalidateTime sql.NullString
		)

		err := rows.Scan(
			&serviceURI,
			&generation,
			&requestTimeRaw,
			&staleTimeRaw,
			&errorCode,
			&errorMessage,
			&statusCode,
			&invalidateTime,
		)
		if err != nil {
			return nil, fmt.Errorf("db: %v", err)
		}

		requestTime, err := time.Parse(time.RFC3339, requestTimeRaw)
		if err != nil {
			return nil, fmt.Errorf("lookup time: %v", err)
		}

		staleTime, err := time.Parse(time.RFC3339, staleTimeRaw)
		if err != nil {
			return nil, fmt.Errorf("lookup time: %v", err)
		}

		summary := &robotstxtdpb.Crawler_ListFiles_Summary{
			ServiceUri:  serviceURI,
			Generation:  generation,
			RequestTime: timestamppb.New(requestTime),
			StaleTime:   timestamppb.New(staleTime),
			Invalidated: invalidateTime.Valid,
		}
		if errorCode.Valid {
			summary.Result = &robotstxtdpb.Crawler_ListFiles_Summary_Error{
				Error: &statuspb.Status{
					Code:    errorCode.Int32,
					Message: errorMessage.String,
				},
			}
		} else {
			summary.Result = &robotstxtdpb.Crawler_ListFiles_Summary_Response_{
				Response: &robotstxtdpb.Crawler_ListFiles_Summary_Response{
					StatusCode: uint32(statusCode.Int32),
				},
			}
		}

		files = append(files, summary)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("db: %v", err)
	}

	res := &robotstxtdpb.Crawler_ListFiles_Response{}
	if uint32(len(files)) > pageSize {
		res.Files = files[:pageSize]
		res.NextPageToken = res.Files[len(res.Files)-1].Generation
	} else {
		res.Files = files
	}

	return res, nil
}
