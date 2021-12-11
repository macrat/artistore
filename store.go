package main

import (
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"
	"unicode/utf8"
)

var (
	ErrRevisionDeleted = errors.New("This revesion has been deleted.")
	ErrNoSuchArtifact  = errors.New("No such artifact on this server.")
)

type Metadata struct {
	Key       string    `json:"key"`
	Revision  int       `json:"revision"`
	Type      string    `json:"type"`
	Timestamp time.Time `json:"-"`
}

type RetainPolicy struct {
	Num    int
	Period time.Duration
}

type Store interface {
	Latest(key string) (revision int, err error)
	Metadata(key string, revision int) (Metadata, error)
	Get(w io.Writer, key string, revision int) error
	Put(key string, r io.Reader) (revision int, err error)
	Sweep()
}

type LocalStore struct {
	Path   string
	Retain RetainPolicy
}

func (s LocalStore) escape(key string) (path string) {
	return url.PathEscape(key)
}

func (s LocalStore) unescape(path string) (key string) {
	x, err := url.PathUnescape(path)
	if err != nil {
		return path
	}
	return x
}

func (s LocalStore) Latest(key string) (revision int, err error) {
	dir, err := os.Open(filepath.Join(s.Path, s.escape(key)))
	if errors.Is(err, os.ErrNotExist) {
		return 0, ErrNoSuchArtifact
	} else if err != nil {
		return
	}
	defer dir.Close()

	xs, err := dir.ReadDir(0)
	if err != nil {
		return
	}

	for _, x := range xs {
		i, err := strconv.Atoi(x.Name())
		if err != nil {
			continue
		}
		if i > revision {
			revision = i
		}
	}

	return
}

type LocalFileReader struct {
	f *os.File
	z *gzip.Reader
}

func (s LocalStore) open(key string, revision int) (LocalFileReader, error) {
	f, err := os.Open(filepath.Join(s.Path, s.escape(key), strconv.Itoa(revision)))
	if err != nil {
		return LocalFileReader{}, err
	}

	z, err := gzip.NewReader(f)
	if err != nil {
		return LocalFileReader{}, err
	}

	return LocalFileReader{f, z}, nil
}

func (f LocalFileReader) Close() error {
	return f.f.Close()
}

func (f LocalFileReader) Metadata() (Metadata, error) {
	var meta Metadata

	if err := json.Unmarshal(f.z.Extra, &meta); err != nil {
		return Metadata{}, err
	}

	meta.Timestamp = f.z.ModTime

	return meta, nil
}

func (f LocalFileReader) Read(p []byte) (int, error) {
	return f.z.Read(p)
}

func (s LocalStore) Metadata(key string, revision int) (Metadata, error) {
	f, err := s.open(key, revision)
	if errors.Is(err, os.ErrNotExist) {
		if latest, err := s.Latest(key); err == nil && revision < latest {
			return Metadata{}, ErrRevisionDeleted
		}
		return Metadata{}, ErrNoSuchArtifact
	} else if err != nil {
		return Metadata{}, nil
	}
	defer f.Close()

	return f.Metadata()
}

func (s LocalStore) Get(w io.Writer, key string, revision int) error {
	f, err := s.open(key, revision)
	if errors.Is(err, os.ErrNotExist) {
		if latest, err := s.Latest(key); err != nil {
			return ErrNoSuchArtifact
		} else if revision < latest {
			return ErrRevisionDeleted
		}
	} else if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(w, f)
	return err
}

type LocalFileWriter struct {
	f *os.File
	z *gzip.Writer
}

func (s LocalStore) create(key string, meta Metadata) (w LocalFileWriter, revision int, err error) {
	revision, _ = s.Latest(key)
	revision++

	if err := os.MkdirAll(filepath.Join(s.Path, s.escape(key)), 0755); err != nil {
		return LocalFileWriter{}, 0, err
	}

	f, err := os.Create(filepath.Join(s.Path, s.escape(key), strconv.Itoa(revision)))
	if err != nil {
		return LocalFileWriter{}, 0, err
	}
	f.Sync()

	z := gzip.NewWriter(f)

	z.Name = key
	z.ModTime = time.Now()
	z.Extra, err = json.Marshal(meta)
	if err != nil {
		return
	}

	return LocalFileWriter{f, z}, revision, nil
}

func (f LocalFileWriter) Close() error {
	if err := f.z.Close(); err != nil {
		return err
	}
	return f.f.Close()
}

func (f LocalFileWriter) Write(p []byte) (int, error) {
	return f.z.Write(p)
}

func detectContentType(data []byte) string {
	typ := http.DetectContentType(data)
	if typ != "application/octet-stream" {
		return typ
	}
	if utf8.Valid(data) {
		return "text/plain; charset=utf-8"
	}
	return typ
}

func (s LocalStore) Put(key string, r io.Reader) (revision int, err error) {
	var head [512]byte
	n, err := r.Read(head[:])
	if err != nil && err != io.EOF {
		return 0, err
	}

	f, revision, err := s.create(key, Metadata{
		Key:      key,
		Revision: revision,
		Type:     detectContentType(head[:n]),
	})
	if err != nil {
		return 0, err
	}
	defer f.Close()

	if _, err = f.Write(head[:n]); err != nil {
		return 0, err
	}

	_, err = io.Copy(f, r)
	if err != nil {
		return 0, err
	}

	go s.sweepByNum(key, revision)

	return revision, nil
}

func (s LocalStore) sweepByNum(key string, latest int) {
	if s.Retain.Num <= 0 {
		return
	}

	dirname := filepath.Join(s.Path, s.escape(key))
	dir, err := os.Open(dirname)
	if err != nil {
		return
	}
	defer dir.Close()

	xs, err := dir.ReadDir(0)
	if err != nil {
		return
	}

	for _, x := range xs {
		rev, err := strconv.Atoi(x.Name())
		if err != nil {
			continue
		}
		if rev <= latest-s.Retain.Num {
			err = os.Remove(filepath.Join(dirname, x.Name()))
			if err != nil {
				PrintErr("ERROR", "failed to sweep old revision %s#%d: %s", key, rev, err)
			} else {
				PrintImportant("SWEEP", "%s#%d", key, rev)
			}
		}
	}
}

func (s LocalStore) sweepByTime(key string) {
	if s.Retain.Period == 0 {
		return
	}

	dirname := filepath.Join(s.Path, s.escape(key))
	dir, err := os.Open(dirname)
	if err != nil {
		return
	}
	defer dir.Close()

	xs, err := dir.ReadDir(0)
	if err != nil {
		return
	}

	latest, err := s.Latest(key)
	if err != nil {
		return
	}

	for _, x := range xs {
		rev, err := strconv.Atoi(x.Name())
		if err != nil {
			continue
		}

		if rev == latest {
			continue
		}

		meta, err := s.Metadata(key, rev)
		if err != nil {
			continue
		}

		if meta.Timestamp.Add(s.Retain.Period).Before(time.Now()) {
			err = os.Remove(filepath.Join(dirname, x.Name()))
			if err != nil {
				PrintErr("ERROR", "failed to sweep old revision %s#%d: %s", key, rev, err)
			} else {
				PrintImportant("SWEEP", "%s#%d", key, rev)
			}
		}
	}
}

func (s LocalStore) Sweep() {
	dir, err := os.Open(s.Path)
	if err != nil {
		return
	}
	defer dir.Close()

	xs, err := dir.ReadDir(0)
	if err != nil {
		return
	}

	for _, x := range xs {
		s.sweepByTime(s.unescape(x.Name()))
	}
}
