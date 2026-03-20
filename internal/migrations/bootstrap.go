package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// Apply executes a frozen bootstrap SQL file or a directory of forward-only SQL
// files against PostgreSQL. It is intentionally simple: no rollback support,
// no version tracking, and no checksum validation.
func Apply(ctx context.Context, dsn, sourcePath string) error {
	files, err := DiscoverFiles(sourcePath)
	if err != nil {
		return err
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}

	for _, file := range files {
		if err := applyFile(ctx, db, file); err != nil {
			return err
		}
		fmt.Printf("✓ Applied %s\n", file)
	}

	fmt.Printf("✓ Applied %d SQL file(s)\n", len(files))
	return nil
}

// DiscoverFiles returns SQL files to apply in lexical order.
// `.undo.` migrations are ignored so the original Omnivore migration directory
// can be used as a forward-only bootstrap source.
func DiscoverFiles(sourcePath string) ([]string, error) {
	info, err := os.Stat(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("stat migration path: %w", err)
	}

	if !info.IsDir() {
		if !isSupportedSQLFile(sourcePath) {
			return nil, fmt.Errorf("migration path must be a .sql file or directory: %s", sourcePath)
		}
		return []string{sourcePath}, nil
	}

	files := make([]string, 0)
	err = filepath.WalkDir(sourcePath, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if isSupportedSQLFile(path) {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk migration path: %w", err)
	}

	sort.Strings(files)
	if len(files) == 0 {
		return nil, fmt.Errorf("no SQL files found in %s", sourcePath)
	}

	return files, nil
}

func applyFile(ctx context.Context, db *sql.DB, path string) error {
	contents, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	sqlText := strings.TrimSpace(string(contents))
	if sqlText == "" {
		return nil
	}

	if _, err := db.ExecContext(ctx, sqlText); err != nil {
		return fmt.Errorf("execute %s: %w", path, err)
	}

	return nil
}

func isSupportedSQLFile(path string) bool {
	if filepath.Ext(path) != ".sql" {
		return false
	}
	return !strings.Contains(filepath.Base(path), ".undo.")
}
