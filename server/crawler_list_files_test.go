package server

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/dpb587/robotstxtd/robotstxtdpb"
)

func TestListFilesPageTokenAndInvalidated(t *testing.T) {
	db := openTestDB(t)
	svc := NewService(testLogger(), db, http.DefaultClient, 0, testBot())

	for n := 1; n <= 201; n++ {
		insertCache(t, db, fmt.Sprintf("http://origin-%d.example", n), fmt.Sprintf("%019d", n), "User-agent: *\nAllow: /\n", 200, nil, false)
	}
	code := int32(14)
	insertCache(t, db, "http://gone.example", fmt.Sprintf("%019d", 202), "", 0, &code, true)

	first, err := svc.ListFiles(context.Background(), &robotstxtdpb.Crawler_ListFiles_Request{
		PageSize: 500,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Files) != 200 {
		t.Fatalf("page size = %d, want 200", len(first.Files))
	}
	if first.Files[0].GetServiceUri() != "http://origin-201.example" {
		t.Fatalf("first origin = %s", first.Files[0].GetServiceUri())
	}
	if first.NextPageToken != first.Files[len(first.Files)-1].GetGeneration() {
		t.Fatalf("next token = %s, last generation = %s", first.NextPageToken, first.Files[len(first.Files)-1].GetGeneration())
	}
	for _, file := range first.Files {
		if file.GetInvalidated() || file.GetServiceUri() == "http://gone.example" {
			t.Fatalf("active page included invalidated row %s", file.GetServiceUri())
		}
		if file.GetResponse().GetStatusCode() != 200 {
			t.Fatalf("status = %d", file.GetResponse().GetStatusCode())
		}
	}

	second, err := svc.ListFiles(context.Background(), &robotstxtdpb.Crawler_ListFiles_Request{
		PageSize:  50,
		PageToken: first.NextPageToken,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Files) != 1 {
		t.Fatalf("second page = %d rows, want 1", len(second.Files))
	}
	if second.Files[0].GetGeneration() != fmt.Sprintf("%019d", 1) {
		t.Fatalf("second generation = %s", second.Files[0].GetGeneration())
	}
	if second.NextPageToken != "" {
		t.Fatalf("unexpected next token %s", second.NextPageToken)
	}

	withInvalidated, err := svc.ListFiles(context.Background(), &robotstxtdpb.Crawler_ListFiles_Request{
		PageSize:           1,
		IncludeInvalidated: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(withInvalidated.Files) != 1 {
		t.Fatalf("filtered page = %d", len(withInvalidated.Files))
	}
	if withInvalidated.Files[0].GetServiceUri() != "http://gone.example" || !withInvalidated.Files[0].GetInvalidated() {
		t.Fatalf("invalidated row = %+v", withInvalidated.Files[0])
	}
	if withInvalidated.Files[0].GetError().GetCode() != code {
		t.Fatalf("error code = %d", withInvalidated.Files[0].GetError().GetCode())
	}
	if withInvalidated.NextPageToken != withInvalidated.Files[0].GetGeneration() {
		t.Fatalf("token = %s", withInvalidated.NextPageToken)
	}

	defaults, err := svc.ListFiles(context.Background(), &robotstxtdpb.Crawler_ListFiles_Request{})
	if err != nil {
		t.Fatal(err)
	}
	if len(defaults.Files) != 50 {
		t.Fatalf("default page size = %d, want 50", len(defaults.Files))
	}
}

func TestPruneDeletesInvalidatedRows(t *testing.T) {
	db := openTestDB(t)
	svc := NewService(testLogger(), db, http.DefaultClient, 0, testBot())

	insertCache(t, db, "http://live.example", fmt.Sprintf("%019d", 2), "User-agent: *\nAllow: /\n", 200, nil, false)
	insertCache(t, db, "http://old.example", fmt.Sprintf("%019d", 1), "", 0, nil, true)

	if err := svc.Prune(); err != nil {
		t.Fatal(err)
	}

	var serviceURI string
	err := db.QueryRow(`SELECT service_uri FROM cache`).Scan(&serviceURI)
	if err != nil {
		t.Fatal(err)
	}
	if serviceURI != "http://live.example" {
		t.Fatalf("remaining row = %s", serviceURI)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM cache`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("rows = %d", count)
	}
}
