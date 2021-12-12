package main

import (
	"compress/gzip"
	"crypto/md5"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
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
	Key       string    `json:"-"`
	Revision  int       `json:"revision"`
	Type      string    `json:"type"`
	Size      int       `json:"size"`
	Hash      string    `json:"md5"`
	Timestamp time.Time `json:"-"`
}

type RetainPolicy struct {
	Num    int
	Period time.Duration
}

type Store interface {
	Latest(key string) (revision int, err error)
	Metadata(key string, revision int) (Metadata, error)
	Get(key string, revision int) (io.ReadSeekCloser, Metadata, error)
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
	f   *os.File
	z   *gzip.Reader
	pos int64
}

func (s LocalStore) open(key string, revision int) (*LocalFileReader, error) {
	f, err := os.Open(filepath.Join(s.Path, s.escape(key), strconv.Itoa(revision)))
	if err != nil {
		return nil, err
	}

	z, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}

	return &LocalFileReader{f, z, 0}, nil
}

func (f *LocalFileReader) Close() error {
	return f.f.Close()
}

func (f *LocalFileReader) Metadata() (Metadata, error) {
	var meta Metadata

	if err := json.Unmarshal(f.z.Extra, &meta); err != nil {
		return Metadata{}, err
	}

	meta.Key = f.z.Name
	meta.Timestamp = f.z.ModTime

	return meta, nil
}

func (f *LocalFileReader) Read(p []byte) (n int, err error) {
	n, err = f.z.Read(p)
	f.pos += int64(n)
	return
}

type DummyWriter struct{}

func (w DummyWriter) Write(p []byte) (int, error) {
	return len(p), nil
}

func (f *LocalFileReader) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		_, err := f.f.Seek(0, io.SeekStart)
		if err != nil {
			return 0, err
		}
		err = f.z.Reset(f.f)
		if err != nil {
			return 0, err
		}
		f.pos = 0
		return io.CopyN(DummyWriter{}, f, offset)
	case io.SeekCurrent:
		return f.Seek(f.pos+offset, io.SeekStart)
	case io.SeekEnd:
		meta, err := f.Metadata()
		if err != nil {
			return 0, err
		}
		return f.Seek(int64(meta.Size)-offset, io.SeekStart)
	}

	return 0, nil
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

func (s LocalStore) Get(key string, revision int) (io.ReadSeekCloser, Metadata, error) {
	f, err := s.open(key, revision)
	if errors.Is(err, os.ErrNotExist) {
		if latest, err := s.Latest(key); err != nil {
			return nil, Metadata{}, ErrNoSuchArtifact
		} else if revision < latest {
			return nil, Metadata{}, ErrRevisionDeleted
		}
	} else if err != nil {
		return nil, Metadata{}, err
	}

	meta, err := f.Metadata()
	if err != nil {
		return nil, Metadata{}, err
	}

	return f, meta, err
}

type LocalFileWriter struct {
	f *os.File
	z *gzip.Writer
}

func (s LocalStore) create(key string) (w LocalFileWriter, revision int, err error) {
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

	return LocalFileWriter{f, z}, revision, nil
}

func (f LocalFileWriter) Close() error {
	if err := f.z.Close(); err != nil {
		return err
	}
	return f.f.Close()
}

func (f LocalFileWriter) SetMetadata(meta Metadata) (err error) {
	f.z.Name = meta.Key
	f.z.ModTime = time.Now()
	f.z.Extra, err = json.Marshal(meta)
	return
}

func (f LocalFileWriter) Write(p []byte) (int, error) {
	return f.z.Write(p)
}

func (f LocalFileWriter) Remove() error {
	if err := f.Close(); err != nil {
		return err
	}
	return os.Remove(f.f.Name())
}

func detectContentType(key string, data []byte) string {
	typ := mime.TypeByExtension(path.Ext(key))
	if typ != "" {
		return typ
	}

	typ = http.DetectContentType(data)
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

	f, revision, err := s.create(key)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	temp, err := NewTempFile()
	if err != nil {
		f.Remove()
		return 0, err
	}
	defer temp.Close()

	if _, err = temp.Write(head[:n]); err != nil {
		f.Remove()
		return 0, err
	}

	_, err = io.Copy(temp, r)
	if err != nil {
		f.Remove()
		return 0, err
	}

	f.SetMetadata(Metadata{
		Key:      key,
		Revision: revision,
		Type:     detectContentType(key, head[:n]),
		Size:     temp.Size(),
		Hash:     temp.Hash(),
	})

	if err = temp.CopyTo(f); err != nil {
		f.Remove()
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

type TempFile struct {
	file *os.File
	hash hash.Hash
	size int
}

func NewTempFile() (*TempFile, error) {
	f, err := os.CreateTemp("", "artistore-temp")
	if err != nil {
		return nil, err
	}
	return &TempFile{f, md5.New(), 0}, nil
}

func (f *TempFile) PrepareToRead() error {
	if err := f.file.Sync(); err != nil {
		return err
	}
	_, err := f.file.Seek(0, os.SEEK_SET)
	return err
}

func (f *TempFile) Write(p []byte) (n int, err error) {
	n, err = f.file.Write(p)
	if err != nil {
		return
	}
	f.size += n

	_, err = f.hash.Write(p)
	return
}

func (f *TempFile) Read(p []byte) (int, error) {
	return f.file.Read(p)
}

func (f *TempFile) CopyTo(w io.Writer) error {
	if err := f.PrepareToRead(); err != nil {
		return err
	}
	_, err := io.Copy(w, f.file)
	return err
}

func (f *TempFile) Size() int {
	return f.size
}

func (f *TempFile) Hash() string {
	return fmt.Sprintf("%032x", f.hash.Sum(nil))
}

func (f *TempFile) Close() error {
	f.file.Close()
	return os.Remove(f.file.Name())
}
