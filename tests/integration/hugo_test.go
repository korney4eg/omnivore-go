package integration_test

import (
"fmt"
"net/http"
"os"
"os/exec"
"path/filepath"
"runtime"
"testing"
"time"
)

const testsitePort = "8765"

// testsiteURL is used everywhere — both from the host (test process) and from
// Docker containers (via extra_hosts: omnivore-testsite:host-gateway in compose).
const testsiteURL = "http://omnivore-testsite:" + testsitePort

// articleTestSlug is set by TestMain (via buildHugo) to a per-run unique value
// so saveUrl never deduplicates against a soft-deleted item from a previous run.
var articleTestSlug string

// repoRoot returns the absolute path to the repository root, derived from this
// file's location (tests/integration/ → ../../).
func repoRoot() string {
_, file, _, _ := runtime.Caller(0)
return filepath.Join(filepath.Dir(file), "..", "..")
}

// TestMain builds a static Hugo site and starts the standalone testsite Docker
// compose before running all integration tests, then tears it down afterward.
//
// Environment variables:
//
//HUGO_DIR  path to Hugo site sources (default: /Users/aliaksei.karneyeu/projects/makvaz.com)
func TestMain(m *testing.M) {
hugoDir := os.Getenv("HUGO_DIR")
if hugoDir == "" {
hugoDir = "/Users/aliaksei.karneyeu/projects/makvaz.com"
}

testsiteDir := filepath.Join(repoRoot(), "testsite")
publicDir := filepath.Join(testsiteDir, "public")

if err := buildHugo(hugoDir, publicDir); err != nil {
fmt.Fprintf(os.Stderr, "hugo build: %v\n", err)
os.Exit(1)
}

if err := startTestsite(testsiteDir); err != nil {
fmt.Fprintf(os.Stderr, "testsite start: %v\n", err)
os.Exit(1)
}

code := m.Run()

stopTestsite(testsiteDir)
os.Exit(code)
}

// enOnlyConfig is a minimal single-language Hugo config. baseURL is set to
// testsiteURL so all generated links (RSS item URLs, etc.) use the hostname
// that Docker containers can reach via extra_hosts.
var enOnlyConfig = `
baseURL = "` + testsiteURL + `"
title = "Makvaz"
theme = ["hugo-notice","jeffprod"]
contentDir = "content/en"
disableKinds = ["section"]

[pagination]
  pagerSize = 5

PygmentsCodeFences = true
PygmentsStyle = "monokai"

enableRobotsTXT = true
rssLimit = 10

[outputFormats]
[outputFormats.RSS]
  mediatype = "application/rss"
  baseName = "feed"

[frontmatter]
  date = [":filename", ":default"]

[permalinks]
  post = "/:year/:month/:day/:slug"

[taxonomies]
  tag = "tags"
  archive = "archives"

[author]
  name = "Aliaksei Karneyeu"

[markup.goldmark.renderer]
  unsafe = true
`

// articleTestPostContent is a synthetic article for the direct save test.
// Its slug is unique per run (timestamp-based) so saveUrl never deduplicates
// against a soft-deleted item from a previous run.
const articleTestPostContent = `---
layout: post
title: Omnivore Article Test
draft: false
tags: [test]
---
This is a synthetic test post used to verify article import in the Omnivore integration test suite.
`

// rssTestPostContent is a synthetic article for the RSS import test. It always
// carries today's date so the queue-processor won't skip it as "old" on a fresh subscription.
const rssTestPostContent = `---
layout: post
title: Omnivore RSS Test Post
draft: false
tags: [test]
---
This is a synthetic test post created by the Omnivore integration test suite.
It always carries today's date so the RSS feed processor treats it as a new item.
`

// buildHugo copies the Hugo source to a temp directory, writes a single-language
// EN config and injects fresh per-run test articles, then builds static output.
func buildHugo(srcDir, publicDir string) error {
if _, err := exec.LookPath("hugo"); err != nil {
return fmt.Errorf("hugo binary not found: %w", err)
}
if _, err := os.Stat(srcDir); err != nil {
return fmt.Errorf("HUGO_DIR %q not found: %w", srcDir, err)
}

tmpDir, err := os.MkdirTemp("", "omnivore-testsite-*")
if err != nil {
return fmt.Errorf("create temp dir: %w", err)
}
defer os.RemoveAll(tmpDir)

if out, err := exec.Command("cp", "-r", srcDir+"/.", tmpDir+"/").CombinedOutput(); err != nil {
return fmt.Errorf("copy site: %w\n%s", err, out)
}

if err := os.WriteFile(filepath.Join(tmpDir, "config.toml"), []byte(enOnlyConfig), 0644); err != nil {
return fmt.Errorf("write config: %w", err)
}

today := time.Now().UTC().Format("2006-01-02")
ts := time.Now().UTC().Format("150405")

// Article test: unique slug per run so saveUrl never re-activates a stale item.
slug := "omnivore-article-test-" + ts
articleTestSlug = slug
articleDir := filepath.Join(tmpDir, "content", "en", "post", today+"-"+slug)
if err := os.MkdirAll(articleDir, 0755); err != nil {
return fmt.Errorf("create article post dir: %w", err)
}
if err := os.WriteFile(filepath.Join(articleDir, "index.md"), []byte(articleTestPostContent), 0644); err != nil {
return fmt.Errorf("write article test post: %w", err)
}

// RSS test: fresh date ensures queue-processor imports it on first subscription.
rssDir := filepath.Join(tmpDir, "content", "en", "post", today+"-omnivore-rss-test")
if err := os.MkdirAll(rssDir, 0755); err != nil {
return fmt.Errorf("create rss post dir: %w", err)
}
if err := os.WriteFile(filepath.Join(rssDir, "index.md"), []byte(rssTestPostContent), 0644); err != nil {
return fmt.Errorf("write rss test post: %w", err)
}

fmt.Printf("hugo: building EN-only static site → %s (article slug: %s)\n", publicDir, slug)
cmd := exec.Command("hugo", "--destination", publicDir, "--environment", "production")
cmd.Dir = tmpDir
cmd.Stdout = os.Stdout
cmd.Stderr = os.Stderr
if err := cmd.Run(); err != nil {
return fmt.Errorf("hugo build failed: %w", err)
}
return nil
}

// startTestsite brings up the testsite nginx container on port 8765.
func startTestsite(dir string) error {
cmd := exec.Command("docker", "compose", "up", "-d", "--wait")
cmd.Dir = dir
cmd.Stdout = os.Stdout
cmd.Stderr = os.Stderr
if err := cmd.Run(); err != nil {
return fmt.Errorf("docker compose up: %w", err)
}

// Health-check from host: localhost:8765 serves the RSS feed.
feedURL := "http://localhost:" + testsitePort + "/feed.xml"
deadline := time.Now().Add(15 * time.Second)
for time.Now().Before(deadline) {
resp, err := http.Get(feedURL)
if err == nil {
resp.Body.Close()
if resp.StatusCode == http.StatusOK {
fmt.Printf("testsite: ready (local: http://localhost:%s, docker: %s)\n", testsitePort, testsiteURL)
return nil
}
}
time.Sleep(300 * time.Millisecond)
}
return fmt.Errorf("testsite did not become ready at %s", feedURL)
}

func stopTestsite(dir string) {
cmd := exec.Command("docker", "compose", "down")
cmd.Dir = dir
_ = cmd.Run()
}
