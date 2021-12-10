package main

import (
	"errors"
	"regexp"
)

var (
	ErrEmptyKey   = errors.New("Invalid key: can not use empty key.")
	ErrSlashKey   = errors.New("Invalid key: key can not have slash at first or last.")
	ErrInvalidKey = errors.New("invalid key: this key contains invalid character.")

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
