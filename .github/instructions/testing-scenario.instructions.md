# Testing Scenario

## Prerequisites

The deploy stack must be brought up fresh with a clean database and empty storage bucket:

```bash
cd deploy && docker compose down -v && docker compose up -d
```

`down -v` removes all volumes (PostgreSQL data, MinIO data, Redis data), ensuring a clean state. The migrate container will recreate the schema and demo user on next startup.

Start the local testsite used by the browser scenario:

```bash
make testsite_start
```

This serves the local Hugo content on `http://localhost:8765` for the host browser and on `http://omnivore-testsite:8765` for Docker containers.

## Steps

### 1. Open the web UI

Navigate to `http://omnivore-deploy:3000`.

### 2. Log in with demo credentials

Click **Continue with Email**, then enter:

- **Email:** `demo@omnivore.work`
- **Password:** `demo_password`

These credentials are created by `setup_db.bash` during the migrate step.

### 3. Add an article

Add the URL `http://omnivore-testsite:8765/2026/01/20/book-learning-ebpf/` using the **Add** button.

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

Add the RSS feed `http://omnivore-testsite:8765/feed.xml` via Subscriptions.

Verify that new posts from the feed are imported and appear in the subscription view. You can also confirm they are returned from Library search with the `in:following` filter.
