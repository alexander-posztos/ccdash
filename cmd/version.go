package cmd

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version, commit, and build date",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("ccdash %s\n  commit:  %s\n  built:   %s\n  runtime: %s %s/%s\n",
			version, commit, date, runtime.Version(), runtime.GOOS, runtime.GOARCH)
		return nil
	},
}
