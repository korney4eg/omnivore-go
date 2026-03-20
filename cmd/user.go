package cmd

import "github.com/spf13/cobra"

var userCmd = &cobra.Command{
	Use:   "user",
	Short: "Manage Omnivore users",
	Long:  "Manage Omnivore users directly from the CLI for self-hosted bootstrap and maintenance tasks.",
}
