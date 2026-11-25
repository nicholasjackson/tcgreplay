package cacheadmin

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/nicholasjackson/tcgreplay/internal/catalog"
	"github.com/nicholasjackson/tcgreplay/internal/importreports"
	"github.com/nicholasjackson/tcgreplay/internal/telemetry"
)

type fakeReports struct {
	list []importreports.Summary
	body map[string][]byte
}

func (f *fakeReports) List(context.Context, int) ([]importreports.Summary, error) { return f.list, nil }

func (f *fakeReports) Get(_ context.Context, id string) ([]byte, error) {
	if b, ok := f.body[id]; ok {
		return b, nil
	}
	return nil, importreports.ErrNotFound
}

// TestImportReportsListedAndDownloaded: with reports kept, the page lists
// them newest first with a download link each; a download is a JSON
// attachment and an admin_action "report" record; an unknown ID is a 404
// with the page.
func TestImportReportsListedAndDownloaded(t *testing.T) {
	ids, err := telemetry.NewIDs("cacheadmin-test-player-id-key-0123456789")
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	rec := telemetry.NewRecorder(&buf, ids, nil)
	s, _ := newService(&fakeCatalog{status: catalog.Status{}}, nil)
	s.Recorder = rec
	const id = "ir-20261125-0123abcd"
	s.Reports = &fakeReports{
		list: []importreports.Summary{{ID: id, Saved: time.Date(2026, 11, 25, 9, 0, 0, 0, time.UTC), Error: "expected two players in Setup"}},
		body: map[string][]byte{id: []byte(`{"id":"` + id + `"}`)},
	}
	ts := serve(t, s)

	status, page := getPage(t, ts)
	if status != http.StatusOK || !strings.Contains(page, `href="/admin/import-reports/`+id+`"`) || !strings.Contains(page, "expected two players in Setup") || !strings.Contains(page, "2026-11-25 09:00:00 UTC") {
		t.Fatalf("page %d:\n%s", status, page)
	}

	resp, err := http.Get(ts.URL + "/admin/import-reports/" + id)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || string(b) != `{"id":"`+id+`"}` ||
		resp.Header.Get("Content-Type") != "application/json" || !strings.Contains(resp.Header.Get("Content-Disposition"), "attachment") {
		t.Fatalf("download %d %v: %s", resp.StatusCode, resp.Header, b)
	}
	if !strings.Contains(buf.String(), `"event":"admin_action"`) || !strings.Contains(buf.String(), `"action":"report"`) {
		t.Fatalf("records: %s", buf.String())
	}

	resp, err = http.Get(ts.URL + "/admin/import-reports/ir-20261125-ffffffff")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown report: %d", resp.StatusCode)
	}
}

// TestNoImportReportsWithoutTheStore: without reports the page has no
// section and the download path is not found.
func TestNoImportReportsWithoutTheStore(t *testing.T) {
	s, _ := newService(&fakeCatalog{status: catalog.Status{}}, nil)
	ts := serve(t, s)
	if _, page := getPage(t, ts); strings.Contains(page, "Import reports") {
		t.Fatal("page offers import reports without a store")
	}
	resp, err := http.Get(ts.URL + "/admin/import-reports/ir-20261125-0123abcd")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("download without a store: %d", resp.StatusCode)
	}
}
