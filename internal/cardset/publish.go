package cardset

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// PublishTarget is where built sets are published: the bucket, or a local
// directory for development and tests.
type PublishTarget interface {
	// Get returns the object under key, or ErrNotFound when there is none.
	Get(ctx context.Context, key string) ([]byte, error)
	// Put writes body under key with its content type and cache lifetime.
	Put(ctx context.Context, key string, body []byte, contentType, cacheControl string) error
	// List returns every key under prefix.
	List(ctx context.Context, prefix string) ([]string, error)
}

// PublishOptions configure Publish.
type PublishOptions struct {
	Dir    string        // the card-set directory (cardsets/)
	Prefix string        // where the layout sits in the target: BucketPrefix in the bucket, "" in a directory
	Target PublishTarget // where to publish
	// Purge drops paths (keys with the prefix) from the CDN's cache. Nil
	// means there is no CDN token: the publish warns that the manifest may
	// stay cached up to the CDN's own lifetime.
	Purge  func(ctx context.Context, paths []string) error
	DryRun bool                 // report what would be uploaded, writing nothing
	Logf   func(string, ...any) // warnings; log.Printf when nil
}

// PublishResult is what a publish uploaded, or would upload on a dry run.
type PublishResult struct {
	Uploaded        []string // set ids uploaded, sorted
	Unchanged       []string // set ids already published as they are, sorted
	ManifestChanged bool     // the manifest was (or would be) rewritten
	Manifest        Manifest // the manifest as published after this run
}

// maxOpenListed bounds how many open items a refused publish names.
const maxOpenListed = 20

// Publish publishes the built sets in opts.Dir to opts.Target. It refuses,
// uploading nothing, while the set check finds an open item or when a set
// the published manifest names is missing from the directory. It uploads,
// gzipped, each set whose hash the published manifest does not already
// give it, under a name made from that hash, then rewrites the manifest
// when anything changed and purges it from the CDN. It never deletes an
// object or overwrites a set object, so every earlier version of a set
// stays retrievable byte for byte.
func Publish(ctx context.Context, opts PublishOptions) (PublishResult, error) {
	logf := opts.Logf
	if logf == nil {
		logf = log.Printf
	}
	var res PublishResult

	items, err := Check(opts.Dir)
	if err != nil {
		return res, err
	}
	if len(items) > 0 {
		lines := make([]string, 0, maxOpenListed)
		for i, it := range items {
			if i == maxOpenListed {
				lines = append(lines, fmt.Sprintf("... and %d more", len(items)-maxOpenListed))
				break
			}
			lines = append(lines, it.String())
		}
		return res, fmt.Errorf("card sets have %d open items (run tcgreplay sets check); nothing published:\n%s", len(items), strings.Join(lines, "\n"))
	}

	local, raws, err := builtSets(opts.Dir)
	if err != nil {
		return res, err
	}

	manifestKey := opts.Prefix + ManifestKey
	remoteBytes, err := opts.Target.Get(ctx, manifestKey)
	remote := Manifest{Format: ManifestFormat, Sets: map[string]ManifestSet{}}
	switch {
	case errors.Is(err, ErrNotFound):
		remoteBytes = nil
	case err != nil:
		return res, fmt.Errorf("read published manifest: %w", err)
	default:
		if remote, err = DecodeManifest(remoteBytes); err != nil {
			return res, fmt.Errorf("read published manifest: %w", err)
		}
	}
	var missing []string
	for id := range remote.Sets {
		if _, ok := local.Sets[id]; !ok {
			missing = append(missing, id)
		}
	}
	if len(missing) > 0 {
		slices.Sort(missing)
		return res, fmt.Errorf("published sets missing from %s: %s; nothing published", filepath.Join(opts.Dir, "sets"), strings.Join(missing, ", "))
	}

	for _, id := range sortedKeys(local.Sets) {
		if remote.Sets[id] == local.Sets[id] {
			res.Unchanged = append(res.Unchanged, id)
		} else {
			res.Uploaded = append(res.Uploaded, id)
		}
	}
	res.Manifest = local
	newBytes, err := EncodeManifest(local)
	if err != nil {
		return res, err
	}
	res.ManifestChanged = !bytes.Equal(newBytes, remoteBytes)
	if opts.DryRun {
		return res, nil
	}

	for _, id := range res.Uploaded {
		key := opts.Prefix + local.Sets[id].Path
		have, err := opts.Target.List(ctx, opts.Prefix+"sets/"+id+"/")
		if err != nil {
			return res, fmt.Errorf("list published set %s: %w", id, err)
		}
		if slices.Contains(have, key) {
			continue // uploaded by an earlier, interrupted publish
		}
		gz, err := gzipBytes(raws[id])
		if err != nil {
			return res, err
		}
		if err := opts.Target.Put(ctx, key, gz, "application/gzip", SetCacheControl); err != nil {
			return res, fmt.Errorf("upload set %s: %w", id, err)
		}
	}
	if !res.ManifestChanged {
		return res, nil
	}
	if err := opts.Target.Put(ctx, manifestKey, newBytes, "application/json", ManifestCacheControl); err != nil {
		return res, fmt.Errorf("upload manifest: %w", err)
	}
	if opts.Purge == nil {
		logf("card sets: no CDN token; the manifest may be cached up to the CDN's TTL")
		return res, nil
	}
	if err := opts.Purge(ctx, []string{manifestKey}); err != nil {
		return res, fmt.Errorf("manifest published, but purging it from the CDN failed: %w", err)
	}
	return res, nil
}

// builtSets reads every <dir>/sets/*.json, returning the manifest that
// publishes them as they are and each set's raw bytes by id.
func builtSets(dir string) (Manifest, map[string][]byte, error) {
	m := Manifest{Format: ManifestFormat, Sets: map[string]ManifestSet{}}
	paths, err := filepath.Glob(filepath.Join(dir, "sets", "*.json"))
	if err != nil {
		return m, nil, err
	}
	if len(paths) == 0 {
		return m, nil, fmt.Errorf("no built sets in %s", filepath.Join(dir, "sets"))
	}
	raws := make(map[string][]byte, len(paths))
	for _, p := range paths {
		id := strings.TrimSuffix(filepath.Base(p), ".json")
		raw, err := os.ReadFile(p)
		if err != nil {
			return m, nil, err
		}
		s, err := Decode(raw)
		if err != nil {
			return m, nil, fmt.Errorf("%s: %w", p, err)
		}
		sum := sha256.Sum256(raw)
		h := hex.EncodeToString(sum[:])
		m.Sets[id] = ManifestSet{Path: SetKey(id, h), SHA256: h, Prints: len(s.Prints), Format: s.Format}
		raws[id] = raw
	}
	return m, raws, nil
}

// gzipBytes compresses b deterministically: best compression, no name and
// a zero modification time, so the same set always gives the same object.
func gzipBytes(b []byte) ([]byte, error) {
	var buf bytes.Buffer
	zw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return nil, err
	}
	if _, err := zw.Write(b); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
