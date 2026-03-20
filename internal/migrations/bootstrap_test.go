package migrations

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDiscoverFilesFromDirectory(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "0002.do.second.sql"))
	mustWriteFile(t, filepath.Join(dir, "0001.do.first.sql"))
	mustWriteFile(t, filepath.Join(dir, "0001.undo.first.sql"))
	mustWriteFile(t, filepath.Join(dir, "schema.sql"))
	mustWriteFile(t, filepath.Join(dir, "notes.txt"))

	got, err := DiscoverFiles(dir)
	if err != nil {
		t.Fatalf("DiscoverFiles returned error: %v", err)
	}

	want := []string{
		filepath.Join(dir, "0001.do.first.sql"),
		filepath.Join(dir, "0002.do.second.sql"),
		filepath.Join(dir, "schema.sql"),
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DiscoverFiles mismatch\nwant: %#v\ngot:  %#v", want, got)
	}
}

func TestDiscoverFilesFromSingleFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	file := filepath.Join(dir, "schema.sql")
	mustWriteFile(t, file)

	got, err := DiscoverFiles(file)
	if err != nil {
		t.Fatalf("DiscoverFiles returned error: %v", err)
	}

	want := []string{file}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DiscoverFiles mismatch\nwant: %#v\ngot:  %#v", want, got)
	}
}

func TestDiscoverFilesRejectsUndoOnlyFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	file := filepath.Join(dir, "0001.undo.first.sql")
	mustWriteFile(t, file)

	if _, err := DiscoverFiles(file); err == nil {
		t.Fatalf("DiscoverFiles should reject undo file")
	}
}

func mustWriteFile(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("SELECT 1;"), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}
