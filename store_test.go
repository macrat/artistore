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
			f, err := store.Get(tt.Key, i+1)
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
