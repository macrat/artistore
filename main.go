package main

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	version = "HEAD"
	commit  = "unknown"
)

var cmd = &cobra.Command{
	Use:     "artistore",
	Short:   "Artistore - A simple artifact store server.",
	Version: version + " (" + commit + ")",
	Example: `  # 1. Generate server secret.
  $ export ARTISTORE_SECRET=$(artistore secret)

  # 2. Start server
  $ artistore serve

  # 3. Publish your artifact
  $ artistore publish bundle.js

  # 4. Use your artifact via http://localhost:3000/bundle.js`,
}

func init() {
	viper.SetEnvPrefix("artistore")
	viper.AutomaticEnv()
}

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(2)
	}
}
