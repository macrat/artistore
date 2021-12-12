package main

import (
	"errors"
	"regexp"
	"strings"
)

var (
	ErrEmptyKey   = errors.New("Invalid key: can not use empty key.")
	ErrSlashKey   = errors.New("Invalid key: slash can not be the first or the last character of key.")
	ErrInvalidKey = errors.New("Invalid key: this key contains invalid character.")

	keyRegexp = regexp.MustCompile(`^[-._~!$&'()*+,;=:@%/a-zA-Z0-9]+$`)
)

func VerifyKey(key string) error {
	if len(key) == 0 {
		return ErrEmptyKey
	}

	if key[0] == '/' || key[len(key)-1] == '/' {
		return ErrSlashKey
	}

	if !keyRegexp.MatchString(key) {
		return ErrInvalidKey
	}

	return nil
}

func KeyPrefixes(key string) []string {
	if !strings.ContainsRune(key, '/') {
		return []string{}
	}

	xs := strings.Split(key, "/")
	xs = xs[:len(xs)-1]
	results := make([]string, len(xs))

	x := ""
	for i := range results {
		x += xs[i] + "/"
		results[len(results)-i-1] = x
	}

	return results
}
