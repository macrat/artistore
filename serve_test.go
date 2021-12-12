package main

import (
	"testing"
)

func TestRangeRequest(t *testing.T) {
	tests := []struct {
		Input  string
		From   int64
		To     int64
		Suffix int64
		Size   int64
		Err    string
	}{
		{"", 0, 0, 0, 0, ""},
		{"bytes=0-10", 0, 10, 0, 10, ""},
		{"bytes=-10", 0, 0, 10, 10, ""},
		{"bytes=10-42", 10, 42, 0, 32, ""},
		{"bytes=10-", 10, 0, 0, -10, ""},
		{"bytes=3-5,10-18", 3, 5, 0, 2, ""},
		{"chunk=0-100", 0, 0, 0, 0, "Unsupported range length type. Please use bytes."},
		{"bytes 0-100", 0, 0, 0, 0, "Unsupported range length type. Please use bytes."},
		{"bytes=a-11", 0, 0, 0, 0, `strconv.ParseInt: parsing "a": invalid syntax`},
		{"bytes=1-b", 1, 0, 0, -1, `strconv.ParseInt: parsing "b": invalid syntax`},
	}

	for _, tt := range tests {
		t.Run(tt.Input, func(t *testing.T) {
			out, err := parseRangeRequest(tt.Input)
			if tt.Err == "" {
				if err != nil {
					t.Errorf("unexpected error: %s", err)
				}
			} else {
				if err.Error() != tt.Err {
					t.Errorf("unexpected error: %s", err)
				}
			}

			if out.From != tt.From {
				t.Errorf("expected from is %d but got %d", tt.From, out.From)
			}

			if out.To != tt.To {
				t.Errorf("expected to is %d but got %d", tt.To, out.To)
			}

			if out.Suffix != tt.Suffix {
				t.Errorf("expected suffix is %d but got %d", tt.Suffix, out.Suffix)
			}

			if out.Size() != tt.Size {
				t.Errorf("expected size is %d but got %d", tt.Size, out.Size())
			}
		})
	}
}
