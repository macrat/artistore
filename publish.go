package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var publishCmd = &cobra.Command{
	Use:   "publish FILE_KEY",
	Short: "Publish an artifact to Artistore",
	Long:  "Publish an artifact to Artistore.",
	Example: `  $ artistore publish library.js
  $ artistore publish bundle.js -f ./build/bundle.js`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		args[0] = path.Clean(args[0])
		if err := VerifyKey(args[0]); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}

		var token Token
		if t := viper.GetString("token"); strings.TrimSpace(t) != "" {
			var err error
			token, err = ParseToken(strings.TrimSpace(t))
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(2)
			}
		} else if s := strings.TrimSpace(viper.GetString("secret")); s != "" {
			secret, err := ParseSecret(s)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(2)
			}
			token, err = NewToken(secret, args[0])
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		} else {
			fmt.Fprintln(os.Stderr, "Either secret or token is required.\nPlease set at least one of --token flag, ARTISTORE_TOKEN environment variable (recommended), --secret flag, or ARTISTORE_SECRET environment variable.")
			os.Exit(2)
		}

		u, err := GetURL(args[0])
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}

		fname, err := cmd.Flags().GetString("file")
		if fname == "" || err != nil {
			fname = args[0]
		}

		file, err := os.Open(fname)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Failed to open file:", fname)
			os.Exit(1)
		}
		defer file.Close()

		msg, err := Publish(u, token, file)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Print(msg)
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

	publishCmd.Flags().StringP("file", "f", "", "The file to publish. (default same as key)")
}

func Publish(u *url.URL, token Token, data io.Reader) (messages string, err error) {
	client := &http.Client{}

	req, err := http.NewRequest("POST", u.String(), data)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Authorization", "bearer "+token.String())

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", errors.New(string(body))
	}
	return string(body), nil
}
