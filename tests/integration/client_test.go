package integration_test

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

// uuidV4 generates a random UUID v4 string.
func uuidV4() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

const (
	defaultBaseURL  = "http://omnivore-deploy"
	defaultEmail    = "demo@omnivore.work"
	defaultPassword = "demo_password"
	defaultUsername = "demo_user"
)

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// omnivoreClient wraps an HTTP client with auth state for the Omnivore API.
type omnivoreClient struct {
	http      *http.Client
	baseURL   string
	username  string
	authToken string // JWT from the auth cookie, sent as Cookie header
}

func newOmnivoreClient(t *testing.T) *omnivoreClient {
	t.Helper()
	return &omnivoreClient{
		http: &http.Client{
			// Do not follow redirects automatically so we can capture the auth
			// cookie from the first 302 response before the second redirect
			// sends a Secure-flagged cookie (which is skipped over plain HTTP).
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
			Timeout: 30 * time.Second,
		},
		baseURL:  getenv("OMNIVORE_URL", defaultBaseURL),
		username: getenv("OMNIVORE_USERNAME", defaultUsername),
	}
}

// login performs email/password login and stores the auth token for later requests.
func (c *omnivoreClient) login(t *testing.T) {
	t.Helper()
	email := getenv("OMNIVORE_EMAIL", defaultEmail)
	password := getenv("OMNIVORE_PASSWORD", defaultPassword)

	form := url.Values{
		"email":    {email},
		"password": {password},
	}
	resp, err := c.http.PostForm(c.baseURL+"/api/auth/email-login", form)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	defer resp.Body.Close()

	// The API responds with 302 + Set-Cookie: auth=<token> (no Secure flag).
	// A second redirect to /api/client/auth would set Secure cookie (HTTP-only),
	// so we stop here and extract the token directly.
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("login: expected 302 redirect, got %d", resp.StatusCode)
	}
	location := resp.Header.Get("Location")
	if strings.Contains(location, "errorCodes") {
		t.Fatalf("login failed, redirect location: %s", location)
	}
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "auth" {
			c.authToken = cookie.Value
			return
		}
	}
	t.Fatal("login: no auth cookie in response headers")
}

// gqlRequest is the body sent to /api/graphql.
type gqlRequest struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables,omitempty"`
}

// gql executes a GraphQL operation and decodes the response into result.
func (c *omnivoreClient) gql(t *testing.T, query string, variables map[string]interface{}, result interface{}) {
	t.Helper()

	body, err := json.Marshal(gqlRequest{Query: query, Variables: variables})
	if err != nil {
		t.Fatalf("gql marshal: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/api/graphql", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("gql new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", "auth="+c.authToken)

	resp, err := c.http.Do(req)
	if err != nil {
		t.Fatalf("gql request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("gql http %d: %s", resp.StatusCode, string(b))
	}

	var envelope struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("gql decode envelope: %v", err)
	}
	if len(envelope.Errors) > 0 {
		msgs := make([]string, len(envelope.Errors))
		for i, e := range envelope.Errors {
			msgs[i] = e.Message
		}
		t.Fatalf("gql errors: %s", strings.Join(msgs, "; "))
	}
	if result != nil {
		if err := json.Unmarshal(envelope.Data, result); err != nil {
			t.Fatalf("gql decode data: %v", err)
		}
	}
}

// pollUntil retries f every interval until it returns true or timeout is reached.
func pollUntil(t *testing.T, timeout, interval time.Duration, msg string, f func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if f() {
			return
		}
		time.Sleep(interval)
	}
	t.Fatalf("timed out after %s waiting for: %s", timeout, msg)
}

// searchResult mirrors a single node from the Search GQL response.
type searchNode struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Slug         string `json:"slug"`
	URL          string `json:"url"`
	State        string `json:"state"`
	WordsCount   int    `json:"wordsCount"`
	Content      string `json:"content"`
	Folder       string `json:"folder"`
	Subscription string `json:"subscription"`
	Labels       []struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Color string `json:"color"`
	} `json:"labels"`
}

const searchQuery = `
  query Search($after: String, $first: Int, $query: String, $includeContent: Boolean) {
    search(first: $first, after: $after, query: $query, includeContent: $includeContent) {
      ... on SearchSuccess {
        edges {
          node {
            id title slug url state wordsCount content folder subscription
            labels { id name color }
          }
        }
      }
      ... on SearchError { errorCodes }
    }
  }`

// search returns all library items matching the optional query string.
// Pass includeContent=true to fetch article HTML bodies.
func (c *omnivoreClient) search(t *testing.T, query string, includeContent bool) []searchNode {
	t.Helper()
	var resp struct {
		Search struct {
			Edges []struct {
				Node searchNode `json:"node"`
			} `json:"edges"`
		} `json:"search"`
	}
	c.gql(t, searchQuery, map[string]interface{}{
		"first":          50,
		"after":          "0",
		"query":          query,
		"includeContent": includeContent,
	}, &resp)

	nodes := make([]searchNode, 0, len(resp.Search.Edges))
	for _, e := range resp.Search.Edges {
		nodes = append(nodes, e.Node)
	}
	return nodes
}

// findByURL returns the first node whose URL contains substr.
func findByURL(nodes []searchNode, substr string) *searchNode {
	for i := range nodes {
		if strings.Contains(nodes[i].URL, substr) {
			return &nodes[i]
		}
	}
	return nil
}

// mustFindByURL fatals if no matching node is found.
func mustFindByURL(t *testing.T, nodes []searchNode, substr string) *searchNode {
	t.Helper()
	n := findByURL(nodes, substr)
	if n == nil {
		titles := make([]string, len(nodes))
		for i, node := range nodes {
			titles[i] = fmt.Sprintf("%q (%s)", node.Title, node.URL)
		}
		t.Fatalf("article with URL containing %q not found in library.\nFound: %s", substr, strings.Join(titles, "\n  "))
	}
	return n
}
