package main

import (
	"testing"
)

func TestParseRangeRequest(t *testing.T) {
	tests := []struct {
		Input string
		From  int64
		To    int64
		Err   string
	}{
		{"", 0, 0, ""},
		{"bytes 0-10", 0, 10, ""},
		{"bytes 10-42", 10, 42, ""},
		{"bytes 3-5,10-18", 3, 5, ""},
		{"chunk 0-100", 0, 0, "Unsupported range length type. Please use bytes."},
		{"bytes a-11", 0, 0, `strconv.ParseInt: parsing "a": invalid syntax`},
		{"bytes 1-b", 1, 0, `strconv.ParseInt: parsing "b": invalid syntax`},
	}

	for _, tt := range tests {
		t.Run(tt.Input, func(t *testing.T) {
			from, to, err := parseRangeRequest(tt.Input)
			if tt.Err == "" {
				if err != nil {
					t.Errorf("unexpected error: %s", err)
				}
			} else {
				if err.Error() != tt.Err {
					t.Errorf("unexpected error: %s", err)
				}
			}

			if from != tt.From {
				t.Errorf("expected from is %d but got %d", tt.From, from)
			}

			if to != tt.To {
				t.Errorf("expected to is %d but got %d", tt.To, to)
			}
		})
	}
}
