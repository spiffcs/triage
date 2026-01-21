# Architecture

## Data Sources

The tool aggregates items from three GitHub API sources:

1. **Unread Notifications** - Your GitHub notification inbox
2. **Review-Requested PRs** - Open PRs where you're a requested reviewer
3. **Authored PRs** - Your own open PRs (to track stale PRs, reviews needed, etc.)

Items are deduplicated when merging these sources, so a PR won't appear twice.

## Data Flow

```
┌─────────────────────┐     ┌─────────────────────┐     ┌─────────────────────┐
│  Notifications API  │     │  Review-Requested   │     │   Authored PRs      │
│   (unread inbox)    │     │    PRs (search)     │     │     (search)        │
└──────────┬──────────┘     └──────────┬──────────┘     └──────────┬──────────┘
           │                           │                           │
           │                           │ (5m cache)                │ (5m cache)
           ▼                           ▼                           ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                              Merge & Deduplicate                            │
└─────────────────────────────────────────────────────────────────────────────┘
           │
           ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                    Enrich with Details (24h cache)                          │
│              (PR size, review state, labels, comments, etc.)                │
└─────────────────────────────────────────────────────────────────────────────┘
           │
           ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                         Score & Prioritize                                  │
│                    (heuristics + optional AI)                               │
└─────────────────────────────────────────────────────────────────────────────┘
           │
           ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                         Filter & Output                                     │
│               (category, type, reason, repo filters)                        │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Caching Strategy

The tool uses a two-tier caching strategy to balance freshness with API efficiency:

| Cache Type | TTL | Purpose |
|------------|-----|---------|
| PR Lists | 5 minutes | Review-requested and authored PR search results |
| Item Details | 24 hours | Issue/PR metadata (labels, size, review state, etc.) |

Cache location: `~/.cache/priority/details/`

The shorter TTL for PR lists ensures you see new review requests quickly, while the longer TTL for details reduces API calls for metadata that changes less frequently.

## Enriched Data

For each item, the tool fetches additional details:

| Field | Description | Applies To |
|-------|-------------|------------|
| `state` | open, closed, or merged | All |
| `author` | Who created the issue/PR | All |
| `labels` | All labels on the item | All |
| `commentCount` | Number of comments | All |
| `assignees` | Who is assigned | All |
| `additions` | Lines added | PRs only |
| `deletions` | Lines removed | PRs only |
| `changedFiles` | Files modified | PRs only |
| `reviewState` | approved, changes_requested, or pending | PRs only |
| `reviewComments` | Number of review comments | PRs only |
| `mergeable` | Whether the PR can be merged | PRs only |
| `draft` | Whether the PR is a draft | PRs only |

## Project Structure

```
priority/
├── cmd/priority/
│   └── main.go              # CLI entry point, command definitions
├── internal/
│   ├── github/
│   │   ├── client.go        # GitHub API client, PR fetching
│   │   ├── cache.go         # Caching layer
│   │   ├── details.go       # Enrichment logic, concurrent workers
│   │   ├── notifications.go # Notification fetching
│   │   └── types.go         # Data structures
│   ├── priority/
│   │   ├── engine.go        # Prioritization engine, filters
│   │   ├── heuristics.go    # Scoring logic
│   │   ├── llm.go           # Claude AI integration
│   │   └── types.go         # Priority types
│   └── output/
│       ├── formatter.go     # Base formatter interface
│       ├── table.go         # Table output
│       ├── json.go          # JSON output
│       └── markdown.go      # Markdown output
└── config/
    └── config.go            # Configuration management
```

## Concurrency

Detail enrichment uses a worker pool pattern for concurrent API requests:

- Default: 20 workers (configurable via `-w` flag)
- First pass checks cache to avoid unnecessary API calls
- Progress callback updates the UI during fetching
- Atomic counters for thread-safe progress tracking

## GitHub API Usage

The tool uses the [google/go-github](https://github.com/google/go-github) library (v57).

Key API endpoints:
- `GET /notifications` - Unread notifications
- `GET /search/issues?q=is:pr+is:open+review-requested:{user}` - PRs awaiting review
- `GET /search/issues?q=is:pr+is:open+author:{user}` - User's open PRs
- `GET /repos/{owner}/{repo}/issues/{number}` - Issue details
- `GET /repos/{owner}/{repo}/pulls/{number}` - PR details
- `GET /repos/{owner}/{repo}/pulls/{number}/reviews` - PR review state
