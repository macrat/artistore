package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	ErrSecretNotSet = errors.New(`Please set ARTISTORE_SECRET environment variable.
You can generate this value using 'artistore secret' command.

$ export ARTISTORE_SECRET=$(artistore secret)`)
	ErrInvalidSecret = errors.New("Invalid secret")
	ErrInvalidToken  = errors.New("Invalid token")

	ErrSeemsToken  = errors.New("Invalid secret: it's seems client token.")
	ErrSeemsSecret = errors.New("Invalid token: it's seems server secret.")
)

var secretCmd = &cobra.Command{
	Use:   "secret",
	Short: "Generate secret",
	Long: `Generate secret.

The secret is used to generate token and verify token.
Please set secret to ARTISTORE_SECRET environment variable of the server.`,
	Example: `  # Generate secret.
  $ export ARTISTORE_SECRET=$(artistore secret)

  # And then, start server.
  $ artistore serve`,
	Args: cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		secret, err := NewSecret()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println(secret)
	},
}

var tokenCmd = &cobra.Command{
	Use:   "token KEY",
	Short: "Generate token",
	Long: `Generate token to publish artifacts to the server.

If the key has slash at the end, it will work as prefix of key.
For example, token that generated for key "hello/" can publish artifacts starts with "hello/" such as "hello/world" or "hello/artistore".`,
	Example: `  # Generate token for bundle.js by secret.
  $ export ARTISTORE_SECRET="your-secret-here"
  $ artistore token prefix/

  # And then, publish an artifact.
  $ artistore publish prefix/your-artifact.dat`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		err := VerifyKey(args[0])
		if err == ErrSlashKey {
			if args[0][0] == '/' {
				fmt.Fprintln(os.Stderr, "Invalid key: slash can not be the first character of key for token.")
				os.Exit(2)
			}
		} else if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}

		secret, err := GetSecret()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}

		token, err := NewToken(secret, args[0])
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println(token)
	},
}

func init() {
	cmd.AddCommand(secretCmd)
	cmd.AddCommand(tokenCmd)

	tokenCmd.Flags().String("secret", "", "Server secret. See also 'artistore help secret'.")
	viper.BindPFlag("secret", tokenCmd.Flags().Lookup("secret"))
}

type Secret []byte

func NewSecret() (Secret, error) {
	var buf [32]byte
	_, err := rand.Read(buf[:])
	return Secret(buf[:]), err
}

func ParseSecret(raw string) (Secret, error) {
	if strings.HasPrefix(raw, "t1:") {
		return nil, ErrSeemsToken
	}
	if len(raw) != 46 || !strings.HasPrefix(raw, "s1:") {
		return nil, ErrInvalidSecret
	}

	var buf [32]byte
	_, err := base64.RawURLEncoding.Decode(buf[:], []byte(raw)[3:])
	return Secret(buf[:]), err
}

func GetSecret() (Secret, error) {
	raw := strings.TrimSpace(viper.GetString("secret"))
	if raw == "" {
		return nil, ErrSecretNotSet
	}

	return ParseSecret(raw)
}

func (s Secret) String() string {
	return "s1:" + base64.RawURLEncoding.EncodeToString(s)
}

type Token []byte

func NewTokenWithSalt(s Secret, key string, salt []byte) Token {
	h := sha256.New224()
	h.Write(s)
	h.Write(salt)
	h.Write([]byte(key))

	var buf [32]byte
	copy(buf[:4], salt)
	copy(buf[4:], h.Sum(nil))
	return Token(buf[:])
}

func NewToken(s Secret, key string) (Token, error) {
	var salt [4]byte
	_, err := rand.Read(salt[:])
	if err != nil {
		return nil, err
	}

	return NewTokenWithSalt(s, key, salt[:]), nil
}

func ParseToken(raw string) (t Token, err error) {
	if strings.HasPrefix(raw, "s1:") {
		return nil, ErrSeemsSecret
	}
	if len(raw) != 46 || !strings.HasPrefix(raw, "t1:") {
		return nil, ErrInvalidToken
	}

	tok, err := base64.RawURLEncoding.DecodeString(raw[3:])
	return Token(tok), err
}

func (t Token) String() string {
	return "t1:" + base64.RawURLEncoding.EncodeToString(t)
}

func (t Token) Salt() []byte {
	return t[:4]
}

func IsCorrentToken(s Secret, t Token, key string) bool {
	if hmac.Equal(NewTokenWithSalt(s, key, t.Salt()), t) {
		return true
	}
	for _, k := range KeyPrefixes(key) {
		if hmac.Equal(NewTokenWithSalt(s, k, t.Salt()), t) {
			return true
		}
	}
	return false
}
