package cmd

import "github.com/spf13/cobra"

var dbCmd = &cobra.Command{
	Use:   "db",
	Short: "Run database utilities",
	Long:  "Run database utilities such as bootstrapping the PostgreSQL schema from SQL files.",
}
