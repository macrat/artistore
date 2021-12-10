package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var getCmd = &cobra.Command{
	Use:   "get FILE_KEY",
	Short: "Get an artifact from Artistore",
	Long:  "Get an artifact from Artistore.",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		u, err := GetURL(args[0])
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}

		if rev, err := cmd.Flags().GetInt("revision"); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		} else if rev > 0 {
			q := u.Query()
			q.Set("rev", strconv.Itoa(rev))
			u.RawQuery = q.Encode()
		}

		resp, err := http.Get(u.String())
		if err != nil {
			fmt.Fprintln(os.Stderr, "Failed to fetch:", err)
			os.Exit(1)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			io.Copy(os.Stdout, resp.Body)
			os.Exit(1)
		}

		output := os.Stdout
		if fname, err := cmd.Flags().GetString("output"); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		} else if fname != "" {
			output, err = os.Create(fname)
			if err != nil {
				fmt.Fprintln(os.Stderr, "Failed to create output file:", err)
				os.Exit(1)
			}
		}

		io.Copy(output, resp.Body)
	},
}

func init() {
	cmd.AddCommand(getCmd)

	getCmd.Flags().String("server", "http://localhost:3000", "URL for Artistore server.")
	viper.BindPFlag("server", getCmd.Flags().Lookup("server"))

	getCmd.Flags().IntP("revision", "r", 0, "Revision of the artifact. (default latest)")
	getCmd.Flags().StringP("output", "o", "", "Output file name. (default stdout)")
}
