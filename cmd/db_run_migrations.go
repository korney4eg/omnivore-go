package cmd

import (
	"context"
	"fmt"

	"github.com/omnivore-app/omnivore/internal/config"
	"github.com/omnivore-app/omnivore/internal/migrations"
	"github.com/spf13/cobra"
)

var migrationFilesPath string

var runMigrationsCmd = &cobra.Command{
	Use:   "run-migrations",
	Short: "Apply bootstrap SQL files to PostgreSQL",
	Long: `Apply a frozen schema snapshot or a directory of forward-only SQL files.

This command is intentionally simpler than the original TypeScript/Postgrator
setup: it runs SQL files in lexical order, ignores .undo. files, and does not
track migration state. It is meant for one-shot self-hosted bootstrap flows.`,
	RunE: runMigrations,
}

func init() {
	dbCmd.AddCommand(runMigrationsCmd)
	runMigrationsCmd.Flags().StringVarP(&migrationFilesPath, "files", "f", "./migrations/", "Path to a .sql file or a directory of .sql files")
}

func runMigrations(cmd *cobra.Command, args []string) error {
	dsn := config.BuildDatabaseURL()
	if dsn == "" {
		return fmt.Errorf("DATABASE_URL or PG_* variables required")
	}

	fmt.Printf("Applying SQL from %s\n", migrationFilesPath)
	return migrations.Apply(context.Background(), dsn, migrationFilesPath)
}
