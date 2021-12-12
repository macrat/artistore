package main

import (
	"bytes"
	"io"
	"testing"
	"time"
)

func TestLocalStore(t *testing.T) {
	store := LocalStore{t.TempDir(), RetainPolicy{2, 0}}

	tests := []struct {
		Key      string
		Revision int
		Data     [][]byte
	}{
		{
			"hello",
			1,
			[][]byte{
				[]byte("hello world"),
			},
		},
		{
			"hello",
			2,
			[][]byte{
				[]byte("hello world"),
				[]byte("this is a test"),
			},
		},
		{
			"world",
			1,
			[][]byte{
				[]byte("second file"),
			},
		},
		{
			"hello",
			3,
			[][]byte{
				nil,
				[]byte("this is a test"),
				[]byte("foo"),
			},
		},
		{
			"hello",
			4,
			[][]byte{
				nil,
				nil,
				[]byte("foo"),
				[]byte("bar"),
			},
		},
	}

	// --- revision 1 ---

	for _, tt := range tests {
		rev, err := store.Put(tt.Key, bytes.NewBuffer(tt.Data[len(tt.Data)-1]))
		if err != nil {
			t.Fatalf("%s#%d: failed to publish: %s", tt.Key, tt.Revision, err)
		}
		if rev != tt.Revision {
			t.Fatalf("%s#%d: revision should be %d but got %d", tt.Key, tt.Revision, tt.Revision, rev)
		}

		rev, err = store.Latest(tt.Key)
		if err != nil {
			t.Fatalf("%s#%d: failed to get revision: %s", tt.Key, tt.Revision, err)
		}
		if rev != tt.Revision {
			t.Fatalf("%s#%d: revision should be %d but got %d", tt.Key, tt.Revision, tt.Revision, rev)
		}

		time.Sleep(10 * time.Millisecond) // Wait for goroutine to remove old revisions.

		for i, data := range tt.Data {
			f, meta, err := store.Get(tt.Key, i+1, RangeRequest{})
			if f != nil {
				defer f.Close()
			}

			if data == nil {
				if err != ErrRevisionDeleted {
					t.Fatalf("%s#%d: revision %d should be removed: error=%s", tt.Key, tt.Revision, i+1, err)
				}
			} else {
				if err != nil {
					t.Fatalf("%s#%d: failed to get revision %d: %s", tt.Key, tt.Revision, i+1, err)
				}

				if meta.Key != tt.Key {
					t.Fatalf("%s#%d: key of #%d should be %s but got %s", tt.Key, tt.Revision, i+1, tt.Key, meta.Key)
				}

				if meta.Revision != i+1 {
					t.Fatalf("%s#%d: revision of #%d should be %d but got %d", tt.Key, tt.Revision, i+1, i+1, meta.Revision)
				}

				if meta.Size != int64(len(data)) {
					t.Fatalf("%s#%d: size of #%d should be %d but got %d", tt.Key, tt.Revision, i+1, len(data), meta.Size)
				}

				buf := &bytes.Buffer{}
				_, err := io.Copy(buf, f)
				if err != nil {
					t.Fatalf("%s#%d: failed to read revision %d: %s", tt.Key, tt.Revision, i+1, err)
				}

				if !bytes.Equal(buf.Bytes(), data) {
					t.Fatalf("%s#%d: unexpected body of revision %d: %q", tt.Key, tt.Revision, i+1, buf)
				}
			}
		}
	}
}

func TestConsumeUntil(t *testing.T) {
	tests := []struct {
		Input  string
		Pos    int64
		Expect string
	}{
		{"hello world", 0, "hello world"},
		{"hello world", 6, "world"},
		{"hello world", 11, ""},
	}

	for _, tt := range tests {
		f := bytes.NewReader([]byte(tt.Input))
		if err := consumeUntil(f, tt.Pos); err != nil {
			t.Errorf("failed to read %d bytes from %q: %s", tt.Pos, tt.Input, err)
		}

		output, err := io.ReadAll(f)
		if err != nil {
			t.Errorf("failed to read remain after read %d bytes from %q: %s", tt.Pos, tt.Input, err)
		}
		if string(output) != tt.Expect {
			t.Errorf("read %d bytes from %q should be %q but got %q", tt.Pos, tt.Input, tt.Expect, string(output))
		}
	}
}
