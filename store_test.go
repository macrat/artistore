package main

import (
	"bytes"
	"testing"
)

func TestLocalStore(t *testing.T) {
	store := LocalStore{t.TempDir()}

	// --- revision 1 ---

	rev, err := store.Put("hello", bytes.NewBuffer([]byte("hello world")))
	if err != nil {
		t.Fatalf("failed to put hello rev 1: %s", err)
	}
	if rev != 1 {
		t.Fatalf("first revision should be 1 but got %d", rev)
	}

	rev, err = store.Latest("hello")
	if err != nil {
		t.Fatalf("failed to get revision of hello: %s", err)
	}
	if rev != 1 {
		t.Fatalf("revision of hello should be 1 but got %d", rev)
	}

	buf := &bytes.Buffer{}
	err = store.Get(buf, "hello", 1)
	if err != nil {
		t.Fatalf("failed to get hello rev 1: %s", err)
	}
	if buf.String() != "hello world" {
		t.Fatalf("unexpected body: %q", buf)
	}

	// --- revision 2 ---

	rev, err = store.Put("hello", bytes.NewBuffer([]byte("this is a test")))
	if err != nil {
		t.Fatalf("failed to put hello rev 2: %s", err)
	}
	if rev != 2 {
		t.Fatalf("second revision should be 2 but got %d", rev)
	}

	rev, err = store.Latest("hello")
	if err != nil {
		t.Fatalf("failed to get revision of hello: %s", err)
	}
	if rev != 2 {
		t.Fatalf("revision of hello should be 2 but got %d", rev)
	}

	buf.Reset()
	err = store.Get(buf, "hello", 2)
	if err != nil {
		t.Fatalf("failed to get hello rev 2: %s", err)
	}
	if buf.String() != "this is a test" {
		t.Fatalf("unexpected body: %q", buf)
	}

	buf.Reset()
	err = store.Get(buf, "hello", 1)
	if err != nil {
		t.Fatalf("failed to get hello rev 1: %s", err)
	}
	if buf.String() != "hello world" {
		t.Fatalf("unexpected body: %q", buf)
	}
}
