package main

import (
	"reflect"
	"testing"
)

func TestKeyPrefixes(t *testing.T) {
	tests := []struct {
		Input  string
		Output []string
	}{
		{"hello", []string{}},
		{"hello/world", []string{"hello/"}},
		{"a/b/c/d", []string{"a/b/c/", "a/b/", "a/"}},
	}

	for _, tt := range tests {
		xs := KeyPrefixes(tt.Input)

		if !reflect.DeepEqual(xs, tt.Output) {
			t.Errorf("%q\nexpected: %v\n but got: %v", tt.Input, tt.Output, xs)
		}
	}
}
