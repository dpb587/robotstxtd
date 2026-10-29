package server

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dpb587/robotstxtd/robotstxtdpb"
)

func TestGetFileSingleflight(t *testing.T) {
	var calls atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/robots.txt" {
			http.NotFound(w, r)
			return
		}
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		io.WriteString(w, "User-agent: *\nAllow: /\n")
	}))
	t.Cleanup(upstream.Close)

	svc := NewService(testLogger(), openTestDB(t), upstream.Client(), 30*time.Second, testBot())
	origin := upstream.URL

	type got struct {
		generation string
		err        error
	}

	var wg sync.WaitGroup
	results := make([]got, 2)
	start := make(chan struct{})
	wg.Add(len(results))
	for i := range results {
		go func() {
			defer wg.Done()
			<-start
			res, err := svc.GetFile(context.Background(), &robotstxtdpb.Crawler_GetFile_Request{
				ServiceUri: origin,
			})
			generation := ""
			if res != nil {
				generation = res.GetAuthority().GetGeneration()
			}
			results[i] = got{generation: generation, err: err}
		}()
	}

	close(start)

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("upstream was not called")
	}

	time.Sleep(200 * time.Millisecond)
	if calls.Load() != 1 {
		t.Fatalf("upstream calls = %d, want 1 while the first request is in flight", calls.Load())
	}

	close(release)
	wg.Wait()

	if results[0].err != nil || results[1].err != nil {
		t.Fatalf("errors: %v %v", results[0].err, results[1].err)
	}
	if results[0].generation == "" || results[0].generation != results[1].generation {
		t.Fatalf("generations = %q %q", results[0].generation, results[1].generation)
	}
	if calls.Load() != 1 {
		t.Fatalf("upstream calls = %d, want 1", calls.Load())
	}

	res, err := svc.GetFile(context.Background(), &robotstxtdpb.Crawler_GetFile_Request{
		ServiceUri: origin,
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("fresh call fetched again, calls = %d", calls.Load())
	}
	if res.GetAuthority().GetGeneration() != results[0].generation {
		t.Fatalf("stored generation = %s, want %s", res.GetAuthority().GetGeneration(), results[0].generation)
	}
}

func TestGetFileErrorReplacesActiveRow(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "User-agent: *\nAllow: /\n")
	}))

	db := openTestDB(t)
	svc := NewService(testLogger(), db, upstream.Client(), 30*time.Second, testBot())
	origin := upstream.URL

	first, err := svc.GetFile(context.Background(), &robotstxtdpb.Crawler_GetFile_Request{
		ServiceUri: origin,
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.GetUpstream().GetResponse().GetStatusCode() != http.StatusOK {
		t.Fatalf("status = %d", first.GetUpstream().GetResponse().GetStatusCode())
	}

	_, err = db.Exec(
		`UPDATE cache SET stale_time = ? WHERE service_uri = ? AND invalidate_time IS NULL`,
		time.Now().UTC().Add(-time.Hour).Format(time.RFC3339),
		origin,
	)
	if err != nil {
		t.Fatal(err)
	}

	upstream.Close()

	second, err := svc.GetFile(context.Background(), &robotstxtdpb.Crawler_GetFile_Request{
		ServiceUri: origin,
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.GetUpstream().GetError() == nil {
		t.Fatal("expected stored upstream error")
	}
	if second.GetAuthority().GetServiceUri() != origin {
		t.Fatalf("service uri = %s", second.GetAuthority().GetServiceUri())
	}

	var activeURI, activeGeneration string
	err = db.QueryRow(
		`SELECT service_uri, generation FROM cache WHERE invalidate_time IS NULL`,
	).Scan(&activeURI, &activeGeneration)
	if err != nil {
		t.Fatal(err)
	}
	if activeURI != origin {
		t.Fatalf("active service_uri = %s", activeURI)
	}
	if activeGeneration != second.GetAuthority().GetGeneration() {
		t.Fatalf("active generation = %s", activeGeneration)
	}

	var invalidatedURI, invalidatedAt string
	err = db.QueryRow(
		`SELECT service_uri, invalidate_time FROM cache WHERE invalidate_time IS NOT NULL`,
	).Scan(&invalidatedURI, &invalidatedAt)
	if err != nil {
		t.Fatal(err)
	}
	if invalidatedURI != origin {
		t.Fatalf("invalidated service_uri = %s", invalidatedURI)
	}
	if _, err := time.Parse(time.RFC3339, invalidatedAt); err != nil {
		t.Fatalf("invalidate_time = %q: %v", invalidatedAt, err)
	}
}
