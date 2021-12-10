package main

import (
	"bytes"
	"testing"
)

func TestSecret(t *testing.T) {
	s1, err := NewSecret()
	if err != nil {
		t.Fatalf("failed to generate secret: %s", err)
	}

	s2, err := ParseSecret(s1.String())
	if err != nil {
		t.Fatalf("failed to parse secret: %s", err)
	}

	if s1.String() != s2.String() {
		t.Errorf("generated secret is different\n%s\n%s", s1, s2)
	}

	_, err = ParseSecret(s1.String()[:10])
	if err != ErrInvalidSecret {
		t.Fatalf("unexpected error: %s", err)
	}

	_, err = ParseSecret(s1.String()[10:])
	if err != ErrInvalidSecret {
		t.Fatalf("unexpected error: %s", err)
	}

	_, err = ParseSecret("t1:12345")
	if err != ErrSeemsToken {
		t.Fatalf("unexpected error: %s", err)
	}
}

func TestToken(t *testing.T) {
	s, err := NewSecret()
	if err != nil {
		t.Fatalf("failed to generate secret: %s", err)
	}
	t.Logf("secret: %s", s)

	t1, err := NewToken(s, "hello")
	if err != nil {
		t.Fatalf("failed to generate token: %s", err)
	}

	t2, err := NewToken(s, "hello")
	if err != nil {
		t.Fatalf("failed to generate token: %s", err)
	}

	if bytes.Equal(t1, t2) {
		t.Errorf("token should different but same: %s", t1)
	}

	tests := []struct {
		name   string
		tok    Token
		key    string
		expect bool
	}{
		{"token1", t1, "hello", true},
		{"token2", t2, "hello", true},
		{"token1", t1, "world", false},
	}

	for _, tt := range tests {
		if ok := IsCorrentToken(s, tt.tok, tt.key); ok != tt.expect {
			t.Errorf("%s - %q: expected %v but got %v", tt.name, tt.key, tt.expect, ok)
		}
	}

	t12, err := ParseToken(t1.String())
	if err != nil {
		t.Fatalf("failed to parse token: %s", err)
	}
	if t1.String() != t12.String() {
		t.Fatalf("parsed token is different\n%s\n%s", t1, t12)
	}

	_, err = ParseToken(t1.String()[:10])
	if err != ErrInvalidToken {
		t.Fatalf("unexpected error: %s", err)
	}

	_, err = ParseToken(t1.String()[10:])
	if err != ErrInvalidToken {
		t.Fatalf("unexpected error: %s", err)
	}

	_, err = ParseToken("s1:12345")
	if err != ErrSeemsSecret {
		t.Fatalf("unexpected error: %s", err)
	}
}
