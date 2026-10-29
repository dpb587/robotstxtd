package server

import (
	"database/sql"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/dpb587/robotstxtd/robotstxtdpb"
	"github.com/dpb587/robotstxtd/server/sqlite3"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func testBot() *robotstxtdpb.UserAgent {
	return &robotstxtdpb.UserAgent{
		Identifier: "Examplebot",
		Version:    "v0.1",
		Url:        "http://bot.example/",
		Email:      "bot@example.com",
	}
}

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sqlite3.Open(filepath.Join(t.TempDir(), "robotstxtd.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Close()
	})

	return db
}

func insertCache(t *testing.T, db *sql.DB, serviceURI, generation, body string, statusCode int, errCode *int32, invalidated bool) {
	t.Helper()

	var invalidateTime any
	if invalidated {
		invalidateTime = time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	}

	var code any
	var message any
	var responseStatus any
	var headers any
	var bodyText any
	if errCode != nil {
		code = *errCode
		message = "service unavailable"
	} else {
		responseStatus = statusCode
		headers = "Content-Type: text/plain"
		bodyText = body
	}

	_, err := db.Exec(
		`
			INSERT INTO cache (
				service_uri,
				generation,
				request_time,
				stale_time,
				error_code,
				error_message,
				response_status_code,
				response_headers,
				response_body_text,
				invalidate_time
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`,
		serviceURI,
		generation,
		time.Now().UTC().Add(-time.Minute).Format(time.RFC3339),
		time.Now().UTC().Add(24*time.Hour).Format(time.RFC3339),
		code,
		message,
		responseStatus,
		headers,
		bodyText,
		invalidateTime,
	)
	if err != nil {
		t.Fatal(err)
	}
}
