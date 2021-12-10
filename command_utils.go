package main

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/viper"
)

func GetURL(key string) (*url.URL, error) {
	server := strings.TrimSpace(viper.GetString("server"))
	if server == "" {
		return nil, errors.New("Server address is required.\nPlease set --server flag or ARTISTORE_SERVER environment variable.")
	}

	u, err := url.Parse(server)
	if err != nil {
		return nil, fmt.Errorf("Invalid server address: %s", err)
	}

	if err := VerifyKey(key); err != nil {
		return nil, err
	}

	u, err = u.Parse("/" + key)
	if err != nil {
		return nil, fmt.Errorf("Invalid server address: %s", err)
	}

	return u, nil
}
