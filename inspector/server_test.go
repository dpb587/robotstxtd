package inspector

import (
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dpb587/robotstxtd/robotstxtdpb"
	"github.com/dpb587/robotstxtd/server"
	"github.com/dpb587/robotstxtd/server/sqlite3"
	"google.golang.org/protobuf/encoding/protojson"
)

func TestInspectorPagesAndAPI(t *testing.T) {
	db, err := sqlite3.Open(filepath.Join(t.TempDir(), "robotstxtd.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	insert(t, db, "http://alpha.example", fmt.Sprintf("%019d", 30), "User-agent: *\nDisallow: /private\n", false)
	insert(t, db, "http://beta.example", fmt.Sprintf("%019d", 20), "User-agent: *\nAllow: /\n", false)
	insert(t, db, "http://gamma.example", fmt.Sprintf("%019d", 40), "User-agent: *\nDisallow: /\n", true)

	svc := server.NewService(slog.New(slog.NewTextHandler(io.Discard, nil)), db, &http.Client{}, time.Second, &robotstxtdpb.UserAgent{
		Identifier: "Examplebot",
		Version:    "v0.1",
		Url:        "http://bot.example/",
		Email:      "bot@example.com",
	})
	handler, err := NewHandler(svc)
	if err != nil {
		t.Fatal(err)
	}

	botPage := get(t, handler, "/")
	if !strings.Contains(botPage, "Examplebot") || !strings.Contains(botPage, "/static/htmx.min.js") || !strings.Contains(botPage, "/static/app.css") {
		t.Fatalf("bot page missing identity or local assets: %s", botPage)
	}
	for _, forbidden := range []string{"cdn.jsdelivr.net", "unpkg.com", "tailwindcss.com"} {
		if strings.Contains(botPage, forbidden) {
			t.Fatalf("page references %s", forbidden)
		}
	}

	filesPage := get(t, handler, "/files?page_size=1")
	if !strings.Contains(filesPage, "http://alpha.example") || strings.Contains(filesPage, "http://beta.example") || strings.Contains(filesPage, "http://gamma.example") {
		t.Fatalf("first page = %s", filesPage)
	}
	if !strings.Contains(filesPage, "Next page") {
		t.Fatal("missing next page control")
	}

	partial := get(t, handler, "/files?partial=1&page_size=1&page_token="+fmt.Sprintf("%019d", 30))
	if strings.Contains(partial, "<html") || !strings.Contains(partial, "http://beta.example") || strings.Contains(partial, "http://gamma.example") {
		t.Fatalf("partial = %s", partial)
	}

	withInvalidated := get(t, handler, "/files?include_invalidated=1&page_size=1")
	if !strings.Contains(withInvalidated, "http://gamma.example") {
		t.Fatalf("invalidated filter = %s", withInvalidated)
	}

	detail := get(t, handler, "/file?service_uri="+url.QueryEscape("http://alpha.example"))
	if !strings.Contains(detail, "Disallow: /private") || !strings.Contains(detail, fmt.Sprintf("%019d", 30)) {
		t.Fatalf("detail = %s", detail)
	}

	invalidate := postForm(t, handler, "/file/invalidate", url.Values{"service_uri": {"http://alpha.example"}}, "")
	if !strings.Contains(invalidate, "Invalidated") {
		t.Fatalf("invalidate = %s", invalidate)
	}

	eval := postForm(t, handler, "/evaluate", url.Values{"uri": {"http://beta.example/page"}}, "true")
	if !strings.Contains(eval, "Allowed") {
		t.Fatalf("evaluate = %s", eval)
	}

	status, listBody := postJSON(t, handler, "/api/ListFiles", `{"pageSize":1,"includeInvalidated":true}`)
	if status != http.StatusOK {
		t.Fatalf("list status %d %s", status, listBody)
	}
	var listed robotstxtdpb.Crawler_ListFiles_Response
	if err := protojson.Unmarshal([]byte(listBody), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Files) != 1 || listed.Files[0].GetServiceUri() != "http://gamma.example" || listed.NextPageToken == "" {
		t.Fatalf("list json = %s", listBody)
	}
	if !strings.Contains(listBody, `"serviceUri"`) || !strings.Contains(listBody, `"nextPageToken"`) {
		t.Fatalf("response is not protojson: %s", listBody)
	}

	_, botBody := postJSON(t, handler, "/api/GetBot", `{}`)
	if !strings.Contains(botBody, `"identifier"`) || !strings.Contains(botBody, "Examplebot") {
		t.Fatalf("bot json = %s", botBody)
	}

	cssStatus, css := getStatus(t, handler, "/static/app.css")
	if cssStatus != http.StatusOK || !strings.Contains(css, "bg-stone-100") {
		t.Fatalf("css status %d len %d", cssStatus, len(css))
	}
	jsStatus, js := getStatus(t, handler, "/static/htmx.min.js")
	if jsStatus != http.StatusOK || !strings.Contains(js, "htmx") {
		t.Fatalf("htmx status %d len %d", jsStatus, len(js))
	}
}

func insert(t *testing.T, db *sql.DB, serviceURI, generation, body string, invalidated bool) {
	t.Helper()

	var invalidateTime any
	if invalidated {
		invalidateTime = time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	}

	_, err := db.Exec(
		`
			INSERT INTO cache (
				service_uri, generation, request_time, stale_time,
				response_status_code, response_headers, response_body_text, invalidate_time
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`,
		serviceURI,
		generation,
		time.Now().UTC().Add(-time.Minute).Format(time.RFC3339),
		time.Now().UTC().Add(24*time.Hour).Format(time.RFC3339),
		200,
		"Content-Type: text/plain",
		body,
		invalidateTime,
	)
	if err != nil {
		t.Fatal(err)
	}
}

func get(t *testing.T, handler http.Handler, path string) string {
	t.Helper()
	status, body := getStatus(t, handler, path)
	if status != http.StatusOK {
		t.Fatalf("%s status %d: %s", path, status, body)
	}
	return body
}

func getStatus(t *testing.T, handler http.Handler, path string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

func postForm(t *testing.T, handler http.Handler, path string, values url.Values, hx string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(values.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if hx != "" {
		req.Header.Set("HX-Request", hx)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("%s status %d: %s", path, rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}

func postJSON(t *testing.T, handler http.Handler, path, body string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}
