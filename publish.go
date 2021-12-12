package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/gosuri/uiprogress"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var publishCmd = &cobra.Command{
	Use:   "publish KEY...",
	Short: "Publish an artifact to Artistore",
	Long:  "Publish an artifact to Artistore.",
	Example: `  $ artistore publish library.js
  $ artistore publish library/*`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		t, err := NewTokenHandler()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}

		var keys []string
		for _, key := range args {
			key = path.Clean(key)

			if err := VerifyKey(key); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(2)
			}

			if stat, err := os.Stat(key); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(2)
			} else if stat.IsDir() {
				fmt.Fprintln(os.Stderr, "skip "+key+" because it is directory.")
				continue
			}

			keys = append(keys, key)
		}

		if ok := PublishAll(t, keys); !ok {
			os.Exit(1)
		}
	},
}

func init() {
	cmd.AddCommand(publishCmd)

	publishCmd.Flags().String("server", "http://localhost:3000", "URL for Artistore server.")
	viper.BindPFlag("server", publishCmd.Flags().Lookup("server"))

	publishCmd.Flags().String("secret", "", "Server secret. See also 'artistore help secret'.")
	viper.BindPFlag("secret", publishCmd.Flags().Lookup("secret"))

	publishCmd.Flags().String("token", "", "Client token. See also 'artistore help token'.")
	viper.BindPFlag("token", publishCmd.Flags().Lookup("token"))
}

type TokenHandler struct {
	Secret Secret
	Token  Token
}

func NewTokenHandler() (h TokenHandler, err error) {
	if t := viper.GetString("token"); strings.TrimSpace(t) != "" {
		h.Token, err = ParseToken(strings.TrimSpace(t))
		if err != nil {
			return
		}
	} else if s := strings.TrimSpace(viper.GetString("secret")); s != "" {
		h.Secret, err = ParseSecret(s)
		if err != nil {
			return
		}
	} else {
		return h, errors.New("Either secret or token is required.\nPlease set at least one of --token flag, ARTISTORE_TOKEN environment variable (recommended), --secret flag, or ARTISTORE_SECRET environment variable.")
	}
	return
}

func (h TokenHandler) TokenFor(key string) (Token, error) {
	if h.Token != nil {
		return h.Token, nil
	}
	return NewToken(h.Secret, key)
}

type ProgressRecorder struct {
	Current  int64
	Total    int64
	Upstream io.Reader
	Report   func(current, total int64)
}

func (r *ProgressRecorder) Read(p []byte) (n int, err error) {
	n, err = r.Upstream.Read(p)
	r.Current += int64(n)
	r.Report(r.Current, r.Total)
	return
}

func PublishArtifact(token Token, key string, progress func(current, total int64)) (location string, err error) {
	u, err := GetURL(key)
	if err != nil {
		return "", err
	}

	f, err := os.Open(key)
	if err != nil {
		return "", err
	}
	defer f.Close()

	r := &ProgressRecorder{Upstream: f, Report: progress}

	stat, err := f.Stat()
	if err != nil {
		return "", err
	}
	r.Total = stat.Size()

	req, err := http.NewRequest("POST", u.String(), r)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Authorization", "bearer "+token.String())

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusCreated {
		return "", errors.New(string(body))
	}

	progress(stat.Size(), stat.Size())
	return strings.TrimSpace(string(body)), nil
}

func PublishAll(t TokenHandler, keys []string) (ok bool) {
	uiprogress.Start()
	defer uiprogress.Stop()

	okStore := atomic.Value{}
	okStore.Store(true)

	var wg sync.WaitGroup
	for _, key := range keys {
		wg.Add(1)

		key := key
		msg := ""
		bar := uiprogress.AddBar(100).PrependFunc(func(b *uiprogress.Bar) string {
			return fmt.Sprintf("%20s", key)
		}).AppendFunc(func(b *uiprogress.Bar) string {
			if msg != "" {
				return msg
			} else {
				return fmt.Sprintf("%d%%", b.Current())
			}
		})
		bar.Width = 20

		go func() {
			defer wg.Done()

			token, err := t.TokenFor(key)
			if err != nil {
				msg = "error: " + err.Error()
				okStore.CompareAndSwap(true, false)
				return
			}
			msg, err = PublishArtifact(token, key, func(current, total int64) {
				bar.Set(int(current * 100 / total))
			})
			if err != nil {
				msg = "error: " + err.Error()
				okStore.CompareAndSwap(true, false)
				return
			}
		}()
	}

	wg.Wait()

	return okStore.Load().(bool)
}
