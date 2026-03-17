# Testing Scenario

## Prerequisites

The deploy stack must be brought up fresh with a clean database and empty storage bucket:

```bash
cd deploy && docker compose down -v && docker compose up -d
```

`down -v` removes all volumes (PostgreSQL data, MinIO data, Redis data), ensuring a clean state. The migrate container will recreate the schema and demo user on next startup.

## Steps

### 1. Open the web UI

Navigate to `http://omnivore-deploy:3000`.

### 2. Log in with demo credentials

Click **Continue with Email**, then enter:

- **Email:** `demo@omnivore.work`
- **Password:** `demo_password`

These credentials are created by `setup_db.bash` during the migrate step.

### 3. Add an article

Add the URL `https://p.umputun.com/en/2026/02/12/ai-agents-2026/` using the **Add** button.

Verify the article appears in the Library.

### 4. Open the imported article

Click the article and verify it is properly displayed (title, body text readable, no blank content).

### 5. Add a label to the article

Add a label to the article and verify the label appears on the article card.

### 6. Remove the label

Remove the label and verify it is no longer shown on the article.

### 7. Remove the article

Delete the article and verify it no longer appears in the Library.

### 8. Add an RSS feed

Add the RSS feed `https://p.umputun.com/index.xml` via Subscriptions.

Verify that new posts from the feed are imported and appear in the Library.
