package integration_test

import (
	"fmt"
	"testing"
	"time"
)

const (
	testLabel = "TestLabel"
)

const archiveArticleURL = "http://omnivore-testsite:8765/2026/01/20/book-learning-ebpf/"

// testArticleURL returns the URL of the fresh per-run test article.
// The slug is injected into the Hugo build with today's date so the URL is
// reachable at http://omnivore-testsite:8765/<year>/<month>/<day>/<slug>/
func testArticleURL() string {
	now := time.Now().UTC()
	return fmt.Sprintf("%s/%04d/%02d/%02d/%s/",
		testsiteURL, now.Year(), int(now.Month()), now.Day(), articleTestSlug)
}

// testRSSFeedURL returns the RSS feed URL of the local Hugo testsite.
func testRSSFeedURL() string { return testsiteURL + "/feed.xml" }

// articleURLFragment matches the per-run article URL in search results.
// Uses the stable prefix shared by all runs.
const articleURLFragment = "omnivore-article-test"

// TestScenario runs a full end-to-end integration test against the Omnivore
// deploy stack. It exercises all major user actions:
//  1. Login
//  2. Add article by URL
//  3. Verify article is imported (state=SUCCEEDED, words present, content readable)
//  4. Add a label to the article
//  5. Verify label appears
//  6. Remove label
//  7. Verify label is gone
//  8. Delete article
//  9. Verify article is removed from library
//  10. Add RSS feed
//  11. Verify RSS posts are imported into library
func TestScenario(t *testing.T) {
	c := newOmnivoreClient(t)

	t.Run("login", func(t *testing.T) {
		c.login(t)
	})

	t.Run("add_article", func(t *testing.T) {
		testAddArticle(t, c)
	})

	t.Run("verify_article_imported", func(t *testing.T) {
		testVerifyArticleImported(t, c)
	})

	t.Run("add_label", func(t *testing.T) {
		testAddLabel(t, c)
	})

	t.Run("verify_label_added", func(t *testing.T) {
		testVerifyLabelAdded(t, c)
	})

	t.Run("remove_label", func(t *testing.T) {
		testRemoveLabel(t, c)
	})

	t.Run("verify_label_removed", func(t *testing.T) {
		testVerifyLabelRemoved(t, c)
	})

	t.Run("delete_article", func(t *testing.T) {
		testDeleteArticle(t, c)
	})

	t.Run("verify_article_deleted", func(t *testing.T) {
		testVerifyArticleDeleted(t, c)
	})

	t.Run("verify_article_in_trash", func(t *testing.T) {
		testVerifyArticleInTrash(t, c)
	})

	t.Run("restore_article_from_trash", func(t *testing.T) {
		testRestoreArticleFromTrash(t, c)
	})

	t.Run("verify_article_restored", func(t *testing.T) {
		testVerifyArticleRestored(t, c)
	})

	t.Run("add_archive_article", func(t *testing.T) {
		testAddArchiveArticle(t, c)
	})

	t.Run("archive_article", func(t *testing.T) {
		testArchiveArticle(t, c)
	})

	t.Run("verify_article_in_archive", func(t *testing.T) {
		testVerifyArticleInArchive(t, c)
	})

	t.Run("verify_archived_article_openable", func(t *testing.T) {
		testVerifyArchivedArticleOpenable(t, c)
	})

	t.Run("unarchive_article", func(t *testing.T) {
		testUnarchiveArticle(t, c)
	})

	t.Run("verify_article_unarchived", func(t *testing.T) {
		testVerifyArticleUnarchived(t, c)
	})

	t.Run("add_rss_feed", func(t *testing.T) {
		testAddRSSFeed(t, c)
	})

	t.Run("verify_rss_posts_imported", func(t *testing.T) {
		testVerifyRSSPostsImported(t, c)
	})
}

// ------------------------------------------------------------------

func testAddArticle(t *testing.T, c *omnivoreClient) {
	t.Helper()
	mutation := `
	  mutation SaveUrl($input: SaveUrlInput!) {
	    saveUrl(input: $input) {
	      ... on SaveSuccess    { url clientRequestId }
	      ... on SaveError      { errorCodes message }
	    }
	  }`

	var resp struct {
		SaveURL struct {
			URL             string   `json:"url"`
			ClientRequestID string   `json:"clientRequestId"`
			ErrorCodes      []string `json:"errorCodes"`
			Message         string   `json:"message"`
		} `json:"saveUrl"`
	}
	c.gql(t, mutation, map[string]interface{}{
		"input": map[string]interface{}{
			"url":             testArticleURL(),
			"clientRequestId": uuidV4(),
			"source":          "add-link",
		},
	}, &resp)

	if len(resp.SaveURL.ErrorCodes) > 0 {
		t.Fatalf("saveUrl returned errors: %v — %s", resp.SaveURL.ErrorCodes, resp.SaveURL.Message)
	}
	t.Logf("article queued: clientRequestId=%s", resp.SaveURL.ClientRequestID)
}

func testVerifyArticleImported(t *testing.T, c *omnivoreClient) {
	t.Helper()
	var article *searchNode
	pollUntil(t, 90*time.Second, 3*time.Second, "article state=SUCCEEDED", func() bool {
		nodes := c.search(t, "", true)
		n := findByURL(nodes, articleURLFragment)
		if n == nil {
			t.Log("article not yet in library, waiting...")
			return false
		}
		t.Logf("article found: state=%s wordsCount=%d", n.State, n.WordsCount)
		article = n
		return n.State == "SUCCEEDED"
	})

	if article.WordsCount == 0 {
		t.Error("article has zero word count — import may be incomplete")
	}
	if article.Content == "" {
		t.Error("article content is empty — HTML body was not stored")
	}
	t.Logf("✓ article imported: title=%q words=%d content_len=%d",
		article.Title, article.WordsCount, len(article.Content))
}

func testAddLabel(t *testing.T, c *omnivoreClient) {
	t.Helper()
	nodes := c.search(t, "", false)
	article := mustFindByURL(t, nodes, articleURLFragment)
	labelID := ensureLabel(t, c, testLabel, "#ff0000")

	mutation := `
	  mutation SetLabels($input: SetLabelsInput!) {
	    setLabels(input: $input) {
	      ... on SetLabelsSuccess { labels { id name color } }
	      ... on SetLabelsError   { errorCodes }
	    }
	  }`

	var resp struct {
		SetLabels struct {
			Labels     []struct{ ID, Name, Color string } `json:"labels"`
			ErrorCodes []string                           `json:"errorCodes"`
		} `json:"setLabels"`
	}
	c.gql(t, mutation, map[string]interface{}{
		"input": map[string]interface{}{
			"pageId":   article.ID,
			"labelIds": []string{labelID},
		},
	}, &resp)

	if len(resp.SetLabels.ErrorCodes) > 0 {
		t.Fatalf("setLabels error: %v", resp.SetLabels.ErrorCodes)
	}
	t.Logf("label applied: %+v", resp.SetLabels.Labels)
}

func testVerifyLabelAdded(t *testing.T, c *omnivoreClient) {
	t.Helper()
	nodes := c.search(t, "", false)
	article := mustFindByURL(t, nodes, articleURLFragment)

	for _, l := range article.Labels {
		if l.Name == testLabel {
			t.Logf("✓ label %q found on article", testLabel)
			return
		}
	}
	t.Errorf("label %q not found on article — got labels: %+v", testLabel, article.Labels)
}

func testRemoveLabel(t *testing.T, c *omnivoreClient) {
	t.Helper()
	nodes := c.search(t, "", false)
	article := mustFindByURL(t, nodes, articleURLFragment)

	mutation := `
	  mutation SetLabels($input: SetLabelsInput!) {
	    setLabels(input: $input) {
	      ... on SetLabelsSuccess { labels { id name } }
	      ... on SetLabelsError   { errorCodes }
	    }
	  }`

	var resp struct {
		SetLabels struct {
			Labels     []struct{ Name string } `json:"labels"`
			ErrorCodes []string                `json:"errorCodes"`
		} `json:"setLabels"`
	}
	c.gql(t, mutation, map[string]interface{}{
		"input": map[string]interface{}{
			"pageId":   article.ID,
			"labelIds": []string{}, // empty list removes all labels
		},
	}, &resp)

	if len(resp.SetLabels.ErrorCodes) > 0 {
		t.Fatalf("setLabels (remove) error: %v", resp.SetLabels.ErrorCodes)
	}
	t.Logf("labels after removal: %+v", resp.SetLabels.Labels)
}

func testVerifyLabelRemoved(t *testing.T, c *omnivoreClient) {
	t.Helper()
	nodes := c.search(t, "", false)
	article := mustFindByURL(t, nodes, articleURLFragment)

	for _, l := range article.Labels {
		if l.Name == testLabel {
			t.Errorf("label %q still present after removal", testLabel)
			return
		}
	}
	t.Logf("✓ label %q removed from article", testLabel)
}

// deleteArticle removes a library item by ID using SetBookmarkArticle(bookmark:false).
func deleteArticle(t *testing.T, c *omnivoreClient, articleID string) {
	t.Helper()
	mutation := `
	  mutation SetBookmarkArticle($input: SetBookmarkArticleInput!) {
	    setBookmarkArticle(input: $input) {
	      ... on SetBookmarkArticleSuccess { bookmarkedArticle { id } }
	      ... on SetBookmarkArticleError   { errorCodes }
	    }
	  }`

	var resp struct {
		SetBookmarkArticle struct {
			BookmarkedArticle struct{ ID string } `json:"bookmarkedArticle"`
			ErrorCodes        []string            `json:"errorCodes"`
		} `json:"setBookmarkArticle"`
	}
	c.gql(t, mutation, map[string]interface{}{
		"input": map[string]interface{}{
			"articleID": articleID,
			"bookmark":  false,
		},
	}, &resp)
	if len(resp.SetBookmarkArticle.ErrorCodes) > 0 {
		t.Fatalf("setBookmarkArticle error: %v", resp.SetBookmarkArticle.ErrorCodes)
	}
}

func testDeleteArticle(t *testing.T, c *omnivoreClient) {
	t.Helper()
	nodes := c.search(t, "", false)
	article := mustFindByURL(t, nodes, articleURLFragment)
	deleteArticle(t, c, article.ID)
	t.Logf("article deleted: id=%s", article.ID)
}

func testVerifyArticleDeleted(t *testing.T, c *omnivoreClient) {
	t.Helper()
	// Give the server a moment to process the delete
	time.Sleep(1 * time.Second)

	nodes := c.search(t, "", false)
	if n := findByURL(nodes, articleURLFragment); n != nil {
		t.Errorf("article still present in library after deletion: id=%s state=%s", n.ID, n.State)
		return
	}
	t.Log("✓ article no longer in library")
}

func testVerifyArticleInTrash(t *testing.T, c *omnivoreClient) {
	t.Helper()
	var article *searchNode
	pollUntil(t, 30*time.Second, 2*time.Second, "deleted article visible in trash", func() bool {
		nodes := c.search(t, "in:trash", false)
		article = findByURL(nodes, articleURLFragment)
		return article != nil
	})
	if article.State != "DELETED" {
		t.Fatalf("deleted article should have state=DELETED in trash, got %s", article.State)
	}
	t.Logf("✓ deleted article is visible in trash: id=%s", article.ID)
}

func restoreArticleFromTrash(t *testing.T, c *omnivoreClient, articleID string) {
	t.Helper()

	mutation := `
	  mutation UpdatePage($input: UpdatePageInput!) {
	    updatePage(input: $input) {
	      ... on UpdatePageSuccess { updatedPage { id state } }
	      ... on UpdatePageError   { errorCodes }
	    }
	  }`

	var resp struct {
		UpdatePage struct {
			UpdatedPage struct {
				ID    string `json:"id"`
				State string `json:"state"`
			} `json:"updatedPage"`
			ErrorCodes []string `json:"errorCodes"`
		} `json:"updatePage"`
	}

	c.gql(t, mutation, map[string]interface{}{
		"input": map[string]interface{}{
			"pageId": articleID,
			"state":  "SUCCEEDED",
		},
	}, &resp)

	if len(resp.UpdatePage.ErrorCodes) > 0 {
		t.Fatalf("updatePage restore error: %v", resp.UpdatePage.ErrorCodes)
	}
	if resp.UpdatePage.UpdatedPage.State != "SUCCEEDED" {
		t.Fatalf("restore should set state=SUCCEEDED, got %s", resp.UpdatePage.UpdatedPage.State)
	}
}

func testRestoreArticleFromTrash(t *testing.T, c *omnivoreClient) {
	t.Helper()

	nodes := c.search(t, "in:trash", false)
	article := mustFindByURL(t, nodes, articleURLFragment)
	restoreArticleFromTrash(t, c, article.ID)
	t.Logf("article restored from trash: id=%s", article.ID)
}

func testVerifyArticleRestored(t *testing.T, c *omnivoreClient) {
	t.Helper()

	pollUntil(t, 30*time.Second, 2*time.Second, "restored article back in library", func() bool {
		nodes := c.search(t, "", false)
		article := findByURL(nodes, articleURLFragment)
		return article != nil && article.State == "SUCCEEDED"
	})

	trashNodes := c.search(t, "in:trash", false)
	if article := findByURL(trashNodes, articleURLFragment); article != nil {
		t.Fatalf("restored article should not remain in trash: id=%s state=%s", article.ID, article.State)
	}

	t.Log("✓ deleted article restored back to library")
}

func testAddArchiveArticle(t *testing.T, c *omnivoreClient) {
	t.Helper()
	mutation := `
	  mutation SaveUrl($input: SaveUrlInput!) {
	    saveUrl(input: $input) {
	      ... on SaveSuccess    { url clientRequestId }
	      ... on SaveError      { errorCodes message }
	    }
	  }`

	var resp struct {
		SaveURL struct {
			URL             string   `json:"url"`
			ClientRequestID string   `json:"clientRequestId"`
			ErrorCodes      []string `json:"errorCodes"`
			Message         string   `json:"message"`
		} `json:"saveUrl"`
	}
	c.gql(t, mutation, map[string]interface{}{
		"input": map[string]interface{}{
			"url":             archiveArticleURL,
			"clientRequestId": uuidV4(),
			"source":          "add-link",
		},
	}, &resp)

	if len(resp.SaveURL.ErrorCodes) > 0 {
		t.Fatalf("saveUrl (archive article) returned errors: %v — %s", resp.SaveURL.ErrorCodes, resp.SaveURL.Message)
	}
	t.Logf("archive candidate queued: clientRequestId=%s", resp.SaveURL.ClientRequestID)
}

func archiveArticle(t *testing.T, c *omnivoreClient, articleID string) {
	t.Helper()
	mutation := `
	  mutation SetLinkArchived($input: ArchiveLinkInput!) {
	    setLinkArchived(input: $input) {
	      ... on ArchiveLinkSuccess { linkId message }
	      ... on ArchiveLinkError   { message errorCodes }
	    }
	  }`

	var resp struct {
		SetLinkArchived struct {
			LinkID     string   `json:"linkId"`
			Message    string   `json:"message"`
			ErrorCodes []string `json:"errorCodes"`
		} `json:"setLinkArchived"`
	}
	c.gql(t, mutation, map[string]interface{}{
		"input": map[string]interface{}{
			"linkId":   articleID,
			"archived": true,
		},
	}, &resp)
	if len(resp.SetLinkArchived.ErrorCodes) > 0 {
		t.Fatalf("setLinkArchived archive error: %v", resp.SetLinkArchived.ErrorCodes)
	}
	if resp.SetLinkArchived.LinkID != articleID {
		t.Fatalf("setLinkArchived archived wrong article: got %s want %s", resp.SetLinkArchived.LinkID, articleID)
	}
}

func testArchiveArticle(t *testing.T, c *omnivoreClient) {
	t.Helper()
	var article *searchNode
	pollUntil(t, 90*time.Second, 3*time.Second, "archive article imported with state=SUCCEEDED", func() bool {
		nodes := c.search(t, "", false)
		for i := range nodes {
			if nodes[i].URL == archiveArticleURL {
				article = &nodes[i]
				return nodes[i].State == "SUCCEEDED"
			}
		}
		return false
	})

	archiveArticle(t, c, article.ID)
	t.Logf("article archived: id=%s", article.ID)
}

func testVerifyArticleInArchive(t *testing.T, c *omnivoreClient) {
	t.Helper()
	var archived *searchNode
	pollUntil(t, 30*time.Second, 2*time.Second, "archived article visible in archive", func() bool {
		nodes := c.search(t, "in:archive", false)
		for i := range nodes {
			if nodes[i].URL == archiveArticleURL {
				archived = &nodes[i]
				return true
			}
		}
		return false
	})
	if archived == nil {
		t.Fatalf("archived article %q not found in archive results", archiveArticleURL)
	}
	if archived.State != "ARCHIVED" {
		t.Fatalf("archived article should have state=ARCHIVED, got %s", archived.State)
	}
	t.Logf("✓ archived article is visible in archive: id=%s", archived.ID)
}

func queryArticleBySlug(t *testing.T, c *omnivoreClient, slug string) (state, content string) {
	t.Helper()

	query := `
	  query GetArticle($username: String!, $slug: String!) {
	    article(username: $username, slug: $slug) {
	      ... on ArticleSuccess {
	        article { slug state content }
	      }
	      ... on ArticleError {
	        errorCodes
	      }
	    }
	  }`

	var resp struct {
		Article struct {
			Article struct {
				Slug    string `json:"slug"`
				State   string `json:"state"`
				Content string `json:"content"`
			} `json:"article"`
			ErrorCodes []string `json:"errorCodes"`
		} `json:"article"`
	}

	c.gql(t, query, map[string]interface{}{
		"username": c.username,
		"slug":     slug,
	}, &resp)

	if len(resp.Article.ErrorCodes) > 0 {
		t.Fatalf("article query returned errors for slug %q: %v", slug, resp.Article.ErrorCodes)
	}

	return resp.Article.Article.State, resp.Article.Article.Content
}

func testVerifyArchivedArticleOpenable(t *testing.T, c *omnivoreClient) {
	t.Helper()

	nodes := c.search(t, "in:archive", false)
	var archived *searchNode
	for i := range nodes {
		if nodes[i].URL == archiveArticleURL {
			archived = &nodes[i]
			break
		}
	}
	if archived == nil {
		t.Fatalf("archived article %q not found for openability check", archiveArticleURL)
	}

	state, content := queryArticleBySlug(t, c, archived.Slug)
	if state != "ARCHIVED" {
		t.Fatalf("archived article query should return state=ARCHIVED, got %s", state)
	}
	if content == "" {
		t.Fatal("archived article content is empty")
	}

	t.Logf("✓ archived article can still be opened: slug=%s content_len=%d", archived.Slug, len(content))
}

func unarchiveArticle(t *testing.T, c *omnivoreClient, articleID string) {
	t.Helper()

	mutation := `
	  mutation SetLinkArchived($input: ArchiveLinkInput!) {
	    setLinkArchived(input: $input) {
	      ... on ArchiveLinkSuccess { linkId message }
	      ... on ArchiveLinkError   { message errorCodes }
	    }
	  }`

	var resp struct {
		SetLinkArchived struct {
			LinkID     string   `json:"linkId"`
			Message    string   `json:"message"`
			ErrorCodes []string `json:"errorCodes"`
		} `json:"setLinkArchived"`
	}

	c.gql(t, mutation, map[string]interface{}{
		"input": map[string]interface{}{
			"linkId":   articleID,
			"archived": false,
		},
	}, &resp)

	if len(resp.SetLinkArchived.ErrorCodes) > 0 {
		t.Fatalf("setLinkArchived unarchive error: %v", resp.SetLinkArchived.ErrorCodes)
	}
	if resp.SetLinkArchived.LinkID != articleID {
		t.Fatalf("setLinkArchived unarchived wrong article: got %s want %s", resp.SetLinkArchived.LinkID, articleID)
	}
}

func testUnarchiveArticle(t *testing.T, c *omnivoreClient) {
	t.Helper()

	nodes := c.search(t, "in:archive", false)
	var archived *searchNode
	for i := range nodes {
		if nodes[i].URL == archiveArticleURL {
			archived = &nodes[i]
			break
		}
	}
	if archived == nil {
		t.Fatalf("archived article %q not found for unarchive", archiveArticleURL)
	}

	unarchiveArticle(t, c, archived.ID)
	t.Logf("article unarchived: id=%s", archived.ID)
}

func testVerifyArticleUnarchived(t *testing.T, c *omnivoreClient) {
	t.Helper()

	pollUntil(t, 30*time.Second, 2*time.Second, "article removed from archive and back in library", func() bool {
		archiveNodes := c.search(t, "in:archive", false)
		if findByURL(archiveNodes, archiveArticleURL) != nil {
			return false
		}
		libraryNodes := c.search(t, "", false)
		article := findByURL(libraryNodes, archiveArticleURL)
		return article != nil && article.State == "SUCCEEDED"
	})

	t.Log("✓ archived article was unarchived back into library")
}

func ensureLabel(t *testing.T, c *omnivoreClient, name, color string) string {
	t.Helper()

	if id := findLabelIDByName(t, c, name); id != "" {
		return id
	}

	mutation := `
	  mutation CreateLabel($input: CreateLabelInput!) {
	    createLabel(input: $input) {
	      ... on CreateLabelSuccess { label { id name color } }
	      ... on CreateLabelError   { errorCodes }
	    }
	  }`

	var resp struct {
		CreateLabel struct {
			Label struct {
				ID string `json:"id"`
			} `json:"label"`
			ErrorCodes []string `json:"errorCodes"`
		} `json:"createLabel"`
	}

	c.gql(t, mutation, map[string]interface{}{
		"input": map[string]interface{}{
			"name":  name,
			"color": color,
		},
	}, &resp)

	if len(resp.CreateLabel.ErrorCodes) > 0 {
		t.Fatalf("createLabel error: %v", resp.CreateLabel.ErrorCodes)
	}

	return resp.CreateLabel.Label.ID
}

func findLabelIDByName(t *testing.T, c *omnivoreClient, name string) string {
	t.Helper()

	query := `
	  query Labels {
	    labels {
	      ... on LabelsSuccess { labels { id name } }
	      ... on LabelsError   { errorCodes }
	    }
	  }`

	var resp struct {
		Labels struct {
			Labels []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"labels"`
			ErrorCodes []string `json:"errorCodes"`
		} `json:"labels"`
	}

	c.gql(t, query, nil, &resp)

	if len(resp.Labels.ErrorCodes) > 0 {
		t.Fatalf("labels query error: %v", resp.Labels.ErrorCodes)
	}

	for _, label := range resp.Labels.Labels {
		if label.Name == name {
			return label.ID
		}
	}

	return ""
}

func testAddRSSFeed(t *testing.T, c *omnivoreClient) {
	t.Helper()
	mutation := `
	  mutation Subscribe($input: SubscribeInput!) {
	    subscribe(input: $input) {
	      ... on SubscribeSuccess { subscriptions { id } }
	      ... on SubscribeError   { errorCodes }
	    }
	  }`

	var resp struct {
		Subscribe struct {
			Subscriptions []struct{ ID string } `json:"subscriptions"`
			ErrorCodes    []string              `json:"errorCodes"`
		} `json:"subscribe"`
	}
	c.gql(t, mutation, map[string]interface{}{
		"input": map[string]interface{}{
			"url":              testRSSFeedURL(),
			"subscriptionType": "RSS",
		},
	}, &resp)

	for _, code := range resp.Subscribe.ErrorCodes {
		if code == "ALREADY_SUBSCRIBED" {
			t.Log("RSS feed already subscribed (non-fresh DB), continuing")
			return
		}
	}
	if len(resp.Subscribe.ErrorCodes) > 0 {
		t.Fatalf("subscribe error: %v", resp.Subscribe.ErrorCodes)
	}
	t.Logf("RSS feed subscribed: %+v", resp.Subscribe.Subscriptions)
}

func testVerifyRSSPostsImported(t *testing.T, c *omnivoreClient) {
	t.Helper()
	pollUntil(t, 90*time.Second, 5*time.Second, "RSS posts imported with state=SUCCEEDED", func() bool {
		// RSS posts land in the "following" folder, query in:following
		nodes := c.search(t, "in:following", false)
		succeeded := 0
		for _, n := range nodes {
			if n.State == "SUCCEEDED" && n.Subscription == testRSSFeedURL() {
				succeeded++
			}
		}
		t.Logf("RSS posts in library: total=%d succeeded=%d", len(nodes), succeeded)
		return succeeded >= 1
	})

	// Verify the imported posts have content
	nodes := c.search(t, "in:following", true)
	for _, n := range nodes {
		if n.State == "SUCCEEDED" && n.Subscription == testRSSFeedURL() {
			if n.WordsCount == 0 {
				t.Errorf("RSS post %q has zero word count", n.Title)
			}
			if n.Content == "" {
				t.Errorf("RSS post %q has empty content", n.Title)
			}
			t.Logf("✓ RSS post imported: title=%q words=%d content_len=%d",
				n.Title, n.WordsCount, len(n.Content))
		}
	}
}
