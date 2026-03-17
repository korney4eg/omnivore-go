# Test Fixtures

Static files used by queue processor unit tests. No running server required.

| File | Used by | Description |
|---|---|---|
| `article.html` | `save-page` job tests | Full HTML of the eBPF book review article from the local Hugo testsite |
| `feed.xml` | `refresh-feed` job tests | RSS 2.0 feed with two stable articles (fixed pubDates) |

## Regenerating

These were generated from the Hugo testsite build (`testsite/public/`).
To regenerate: `make testsite_start` then copy updated files here.
The `feed.xml` uses fixed `pubDate` values so tests are not date-sensitive.
