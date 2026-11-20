package cardset

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/nicholasjackson/tcgreplay/internal/spaces"
)

// SpacesTarget publishes to a Spaces bucket. Every object is written
// public-read, so playmats fetch the layout anonymously.
type SpacesTarget struct {
	C *spaces.Client
}

// Get returns the object under key, or ErrNotFound.
func (t SpacesTarget) Get(ctx context.Context, key string) ([]byte, error) {
	b, err := t.C.Get(ctx, key)
	if errors.Is(err, spaces.ErrNotFound) {
		return nil, ErrNotFound
	}
	return b, err
}

// Put writes body under key, public-read, with its content type and cache
// lifetime.
func (t SpacesTarget) Put(ctx context.Context, key string, body []byte, contentType, cacheControl string) error {
	return t.C.Put(ctx, key, body, spaces.PutOptions{ContentType: contentType, CacheControl: cacheControl, Public: true})
}

// List returns every key under prefix, paging through the listing.
func (t SpacesTarget) List(ctx context.Context, prefix string) ([]string, error) {
	var keys []string
	token := ""
	for {
		page, err := t.C.List(ctx, prefix, token)
		if err != nil {
			return keys, err
		}
		keys = append(keys, page.Keys...)
		if page.Next == "" {
			return keys, nil
		}
		token = page.Next
	}
}

// DirTarget publishes to a local directory, Dir/<key>, for development and
// tests; a playmat can load the result with the directory as its base. It
// ignores content types and cache lifetimes.
type DirTarget struct {
	Dir string
}

func (t DirTarget) path(key string) string {
	return filepath.Join(t.Dir, filepath.FromSlash(key))
}

// Get returns the file under key, or ErrNotFound.
func (t DirTarget) Get(_ context.Context, key string) ([]byte, error) {
	b, err := os.ReadFile(t.path(key))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNotFound
	}
	return b, err
}

// Put writes body under key through a temporary file, so an interrupted
// write never leaves half an object.
func (t DirTarget) Put(_ context.Context, key string, body []byte, _, _ string) error {
	return writeFile(t.path(key), body)
}

// List returns every key under prefix, in key order, leaving out the
// temporary files of writes in progress.
func (t DirTarget) List(_ context.Context, prefix string) ([]string, error) {
	root := t.Dir
	if d := path.Dir(prefix); d != "." {
		root = t.path(d)
	}
	var keys []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if d.IsDir() || strings.HasPrefix(d.Name(), ".") {
			return nil
		}
		rel, err := filepath.Rel(t.Dir, p)
		if err != nil {
			return err
		}
		if key := filepath.ToSlash(rel); strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
		return nil
	})
	slices.Sort(keys)
	return keys, err
}
