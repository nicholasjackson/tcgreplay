package cardset

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/nicholasjackson/tcgreplay/internal/spaces"
	"github.com/nicholasjackson/tcgreplay/internal/spaces/spacestest"
)

// publishDir writes two clean built sets, tst and two, and returns their
// card-set directory.
func publishDir(t *testing.T) string {
	t.Helper()
	dir := cleanDir(t)
	s := testSet()
	s.Set.ID = "two"
	s.Set.Text = map[string]SetText{"en": {Name: "Second Set"}}
	for i := range s.Prints {
		s.Prints[i].ID = strings.Replace(s.Prints[i].ID, "tst-", "two-", 1)
		s.Prints[i].Images = []Image{{Source: "tcgdex", URL: "https://assets.tcgdex.net/y"}}
	}
	if err := Write(filepath.Join(dir, "sets", "two.json"), s); err != nil {
		t.Fatal(err)
	}
	return dir
}

// changeSet rewrites one print's HP in a built set.
func changeSet(t *testing.T, dir, id string, hp int) {
	t.Helper()
	path := filepath.Join(dir, "sets", id+".json")
	s, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	s.Prints[0].HP = hp
	if err := Write(path, s); err != nil {
		t.Fatal(err)
	}
}

type logs struct{ lines []string }

func (l *logs) logf(format string, args ...any) {
	l.lines = append(l.lines, fmt.Sprintf(format, args...))
}

func bucketOpts(t *testing.T, srv *spacestest.Server, dir string) PublishOptions {
	return PublishOptions{Dir: dir, Prefix: BucketPrefix, Target: SpacesTarget{C: srv.Client(t)}, Logf: (&logs{}).logf}
}

func gunzip(t *testing.T, b []byte) []byte {
	t.Helper()
	zr, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func puts(srv *spacestest.Server) []string {
	var out []string
	for _, k := range srv.Keys("") {
		if srv.Requests("PUT", k) > 0 {
			out = append(out, k)
		}
	}
	return out
}

func TestPublishFirstUploadsEverySet(t *testing.T) {
	dir := publishDir(t)
	srv := spacestest.New(t)
	res, err := Publish(context.Background(), bucketOpts(t, srv, dir))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(res.Uploaded, []string{"tst", "two"}) || len(res.Unchanged) != 0 || !res.ManifestChanged {
		t.Fatalf("result %+v", res)
	}

	mb, ok := srv.Object("cardsets/manifest.json")
	if !ok {
		t.Fatal("no manifest published")
	}
	m, err := DecodeManifest(mb)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Sets) != 2 || m.Sets["tst"].Prints != 4 || m.Sets["tst"].Format != 1 {
		t.Fatalf("manifest %+v", m)
	}
	h := srv.Header("cardsets/manifest.json")
	if h.Get("Content-Type") != "application/json" || h.Get("Cache-Control") != "public, max-age=60" || !srv.Public("cardsets/manifest.json") {
		t.Errorf("manifest headers %v public=%v", h, srv.Public("cardsets/manifest.json"))
	}
	for _, id := range []string{"tst", "two"} {
		e := m.Sets[id]
		if !strings.HasPrefix(e.Path, "sets/"+id+"/") || !strings.HasSuffix(e.Path, ".json.gz") || len(e.Path) != len("sets/"+id+"/")+16+len(".json.gz") {
			t.Errorf("%s path %q", id, e.Path)
		}
		key := "cardsets/" + e.Path
		gz, ok := srv.Object(key)
		if !ok {
			t.Fatalf("no object %s", key)
		}
		raw, _ := os.ReadFile(filepath.Join(dir, "sets", id+".json"))
		if !bytes.Equal(gunzip(t, gz), raw) {
			t.Errorf("%s does not decompress to the committed file", key)
		}
		h := srv.Header(key)
		if h.Get("Content-Type") != "application/gzip" || h.Get("Cache-Control") != "public, max-age=31536000, immutable" || !srv.Public(key) {
			t.Errorf("%s headers %v public=%v", key, h, srv.Public(key))
		}
	}
}

func TestPublishOneChangedSet(t *testing.T) {
	dir := publishDir(t)
	srv := spacestest.New(t)
	opts := bucketOpts(t, srv, dir)
	if _, err := Publish(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	mb, _ := srv.Object("cardsets/manifest.json")
	m1, _ := DecodeManifest(mb)
	oldKey := "cardsets/" + m1.Sets["tst"].Path
	oldBytes, _ := srv.Object(oldKey)
	srv.Reset()

	changeSet(t, dir, "tst", 90)
	res, err := Publish(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(res.Uploaded, []string{"tst"}) || !slices.Equal(res.Unchanged, []string{"two"}) || !res.ManifestChanged {
		t.Fatalf("result %+v", res)
	}
	mb, _ = srv.Object("cardsets/manifest.json")
	m2, _ := DecodeManifest(mb)
	newKey := "cardsets/" + m2.Sets["tst"].Path
	if newKey == oldKey {
		t.Fatal("changed set kept its object name")
	}
	if got := puts(srv); !slices.Equal(got, []string{"cardsets/manifest.json", newKey}) {
		t.Errorf("PUTs %v, want the manifest and %s only", got, newKey)
	}
	if b, ok := srv.Object(oldKey); !ok || !bytes.Equal(b, oldBytes) {
		t.Error("the set's earlier object is gone or changed")
	}
	if m2.Sets["two"] != m1.Sets["two"] {
		t.Error("unchanged set's entry changed")
	}
}

func TestPublishUnchangedUploadsNothing(t *testing.T) {
	dir := publishDir(t)
	srv := spacestest.New(t)
	opts := bucketOpts(t, srv, dir)
	if _, err := Publish(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	first, _ := srv.Object("cardsets/manifest.json")
	srv.Reset()
	res, err := Publish(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Uploaded) != 0 || res.ManifestChanged || !slices.Equal(res.Unchanged, []string{"tst", "two"}) {
		t.Fatalf("result %+v", res)
	}
	if got := puts(srv); len(got) != 0 {
		t.Errorf("unchanged publish PUT %v", got)
	}
	again, _ := srv.Object("cardsets/manifest.json")
	if !bytes.Equal(first, again) {
		t.Error("manifest bytes changed")
	}
}

func TestPublishRefuses(t *testing.T) {
	t.Run("open check item", func(t *testing.T) {
		dir := publishDir(t)
		writeJSON(t, filepath.Join(dir, "reports", "tst.json"), `{"set":"tst","summary":{},"items":[{"card":"tst-002","kind":"disagreement","field":"/retreat","status":"open"}]}`)
		srv := spacestest.New(t)
		_, err := Publish(context.Background(), bucketOpts(t, srv, dir))
		if err == nil || !strings.Contains(err.Error(), "1 open items") || !strings.Contains(err.Error(), "tst tst-002 disagreement /retreat") {
			t.Fatalf("err %v", err)
		}
		if got := puts(srv); len(got) != 0 {
			t.Errorf("PUT %v", got)
		}
	})
	t.Run("published set missing", func(t *testing.T) {
		dir := publishDir(t)
		srv := spacestest.New(t)
		opts := bucketOpts(t, srv, dir)
		if _, err := Publish(context.Background(), opts); err != nil {
			t.Fatal(err)
		}
		srv.Reset()
		os.Remove(filepath.Join(dir, "sets", "two.json"))
		changeSet(t, dir, "tst", 90)
		_, err := Publish(context.Background(), opts)
		if err == nil || !strings.Contains(err.Error(), "missing") || !strings.Contains(err.Error(), "two") {
			t.Fatalf("err %v", err)
		}
		if got := puts(srv); len(got) != 0 {
			t.Errorf("PUT %v", got)
		}
	})
}

func TestPublishPurgesManifest(t *testing.T) {
	dir := publishDir(t)
	srv := spacestest.New(t)
	cdn := spacestest.NewCDN(t, "test-bucket.lon1.digitaloceanspaces.com", "tok")
	purger := spaces.NewCDN(spaces.Config{Endpoint: "https://lon1.digitaloceanspaces.com", Bucket: "test-bucket"}, "tok")
	purger.API = cdn.URL
	opts := bucketOpts(t, srv, dir)
	opts.Purge = purger.Purge
	if _, err := Publish(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	if p := cdn.Purges(); len(p) != 1 || !slices.Equal(p[0], []string{"cardsets/manifest.json"}) {
		t.Errorf("purges %v", p)
	}
	// Nothing changed: no manifest write, so no purge.
	if _, err := Publish(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	if p := cdn.Purges(); len(p) != 1 {
		t.Errorf("unchanged publish purged: %v", p)
	}
	// A failed purge is reported after the manifest is written.
	cdn.SetFail(true)
	changeSet(t, dir, "two", 50)
	_, err := Publish(context.Background(), opts)
	if err == nil || !strings.Contains(err.Error(), "purging") {
		t.Errorf("failed purge: err %v", err)
	}
}

func TestPublishWarnsWithoutCDNToken(t *testing.T) {
	dir := publishDir(t)
	srv := spacestest.New(t)
	l := &logs{}
	opts := bucketOpts(t, srv, dir)
	opts.Logf = l.logf
	if _, err := Publish(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	if len(l.lines) != 1 || !strings.Contains(l.lines[0], "no CDN token") {
		t.Errorf("logs %q", l.lines)
	}
}

func TestPublishDryRunWritesNothing(t *testing.T) {
	dir := publishDir(t)
	srv := spacestest.New(t)
	opts := bucketOpts(t, srv, dir)
	opts.DryRun = true
	res, err := Publish(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(res.Uploaded, []string{"tst", "two"}) || !res.ManifestChanged {
		t.Fatalf("result %+v", res)
	}
	if got := puts(srv); len(got) != 0 {
		t.Errorf("dry run PUT %v", got)
	}
}

func TestPublishResumesInterruptedUpload(t *testing.T) {
	dir := publishDir(t)
	srv := spacestest.New(t)
	opts := bucketOpts(t, srv, dir)
	// An earlier publish uploaded tst's object but died before the manifest.
	m, raws, err := builtSets(dir)
	if err != nil {
		t.Fatal(err)
	}
	gz, _ := gzipBytes(raws["tst"])
	srv.SetObject("cardsets/"+m.Sets["tst"].Path, gz)
	srv.Reset()
	if _, err := Publish(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	if srv.Requests("PUT", "cardsets/"+m.Sets["tst"].Path) != 0 {
		t.Error("an existing set object was written again")
	}
	if _, ok := srv.Object("cardsets/" + m.Sets["two"].Path); !ok {
		t.Error("two not uploaded")
	}
}

func TestPublishDeterministic(t *testing.T) {
	dir := publishDir(t)
	a, b := t.TempDir(), t.TempDir()
	for _, to := range []string{a, b} {
		if _, err := Publish(context.Background(), PublishOptions{Dir: dir, Target: DirTarget{Dir: to}, Logf: (&logs{}).logf}); err != nil {
			t.Fatal(err)
		}
	}
	ma, _ := os.ReadFile(filepath.Join(a, "manifest.json"))
	mb, _ := os.ReadFile(filepath.Join(b, "manifest.json"))
	if len(ma) == 0 || !bytes.Equal(ma, mb) {
		t.Error("manifests differ between identical publishes")
	}
	m, _ := DecodeManifest(ma)
	for _, e := range m.Sets {
		ga, _ := os.ReadFile(filepath.Join(a, e.Path))
		gb, _ := os.ReadFile(filepath.Join(b, e.Path))
		if len(ga) == 0 || !bytes.Equal(ga, gb) {
			t.Errorf("%s differs between identical publishes", e.Path)
		}
	}
}

func TestPublishToDirectoryRoundTrip(t *testing.T) {
	dir := publishDir(t)
	to := t.TempDir()
	if _, err := Publish(context.Background(), PublishOptions{Dir: dir, Target: DirTarget{Dir: to}, Logf: (&logs{}).logf}); err != nil {
		t.Fatal(err)
	}
	mb, err := os.ReadFile(filepath.Join(to, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := DecodeManifest(mb)
	if err != nil {
		t.Fatal(err)
	}
	for id, e := range m.Sets {
		gz, err := os.ReadFile(filepath.Join(to, filepath.FromSlash(e.Path)))
		if err != nil {
			t.Fatal(err)
		}
		got, err := Decode(gunzip(t, gz))
		if err != nil {
			t.Fatal(err)
		}
		want, _ := Read(filepath.Join(dir, "sets", id+".json"))
		gb, _ := Marshal(got)
		wb, _ := Marshal(want)
		if !bytes.Equal(gb, wb) {
			t.Errorf("%s does not round-trip", id)
		}
	}
	keys, err := DirTarget{Dir: to}.List(context.Background(), "sets/tst/")
	if err != nil || len(keys) != 1 || keys[0] != m.Sets["tst"].Path {
		t.Errorf("List = %v, %v", keys, err)
	}
}

func TestDecodeManifestRefusesUnknownFormat(t *testing.T) {
	if _, err := DecodeManifest([]byte(`{"format":2,"sets":{}}`)); err == nil || !strings.Contains(err.Error(), "format 2") {
		t.Errorf("err %v", err)
	}
	if _, err := DecodeManifest([]byte(`{"format":1,"sets":{"x":{"path":"../x","sha256":"zz"}}}`)); err == nil {
		t.Error("bad entry accepted")
	}
}
