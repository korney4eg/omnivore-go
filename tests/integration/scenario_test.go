package integration_test

import (
	"fmt"
	"testing"
	"time"
)

const (
	testLabel = "TestLabel"
)

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
			URL              string   `json:"url"`
			ClientRequestID  string   `json:"clientRequestId"`
			ErrorCodes       []string `json:"errorCodes"`
			Message          string   `json:"message"`
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
			"pageId": article.ID,
			"labels": []map[string]interface{}{
				{"name": testLabel, "color": "#ff0000"},
			},
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
			ErrorCodes []string               `json:"errorCodes"`
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
