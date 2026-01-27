# Architecture

## Data Sources

The tool aggregates items from four GitHub API sources:

1. **Unread Notifications** - Your GitHub notification inbox
2. **Review-Requested PRs** - Open PRs where you're a requested reviewer
3. **Authored PRs** - Your own open PRs (to track stale PRs, reviews needed, etc.)
4. **Assigned Issues** - Open issues assigned to you

Items are deduplicated when merging these sources, so an item won't appear twice.

## Data Flow

```
┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐
│  Notifications   │  │ Review-Requested │  │  Authored PRs    │  │ Assigned Issues  │
│  (Core API)      │  │  PRs (Search)    │  │    (Search)      │  │    (Search)      │
└────────┬─────────┘  └────────┬─────────┘  └────────┬─────────┘  └────────┬─────────┘
         │ (1h cache)          │ (5m cache)          │ (5m cache)          │ (5m cache)
         ▼                     ▼                     ▼                     ▼
┌─────────────────────────────────────────────────────────────────────────────────────┐
│                              Merge & Deduplicate                                    │
└─────────────────────────────────────────────────────────────────────────────────────┘
                                        │
                                        ▼
┌─────────────────────────────────────────────────────────────────────────────────────┐
│                    Enrich with Details (GraphQL batch, 24h cache)                   │
│              (PR size, review state, CI status, labels, comments, etc.)             │
└─────────────────────────────────────────────────────────────────────────────────────┘
                                        │
                                        ▼
┌─────────────────────────────────────────────────────────────────────────────────────┐
│                              Score & Prioritize                                     │
└─────────────────────────────────────────────────────────────────────────────────────┘
                                        │
                                        ▼
┌─────────────────────────────────────────────────────────────────────────────────────┐
│                               Filter & Output                                       │
└─────────────────────────────────────────────────────────────────────────────────────┘
```

## Caching Strategy

The tool uses a three-tier caching strategy to balance freshness with API efficiency:

| Cache Type | TTL | Purpose |
|------------|-----|---------|
| PR/Issue Lists | 5 minutes | Review-requested PRs, authored PRs, assigned issues |
| Notification Lists | 1 hour | Unread notifications (with incremental updates) |
| Item Details | 24 hours | Issue/PR metadata (labels, size, review state, CI status, etc.) |

Cache location: `~/.cache/triage/details/`

The shorter TTL for PR/issue lists ensures you see new review requests quickly. Notification lists use incremental fetching - only new notifications since last fetch are retrieved and merged with cached data. The longer TTL for details reduces API calls for metadata that changes less frequently.

## Enriched Data

For each item, the tool fetches additional details via GraphQL:

| Field | Description | Applies To |
|-------|-------------|------------|
| `state` | open, closed, or merged | All |
| `author` | Who created the issue/PR | All |
| `labels` | All labels on the item | All |
| `commentCount` | Number of comments | All |
| `assignees` | Who is assigned | All |
| `lastCommenter` | Most recent commenter | Issues only |
| `additions` | Lines added | PRs only |
| `deletions` | Lines removed | PRs only |
| `changedFiles` | Files modified | PRs only |
| `reviewState` | approved, changes_requested, or pending | PRs only |
| `ciStatus` | success, failure, or pending | PRs only |
| `mergeable` | Whether the PR can be merged | PRs only |
| `draft` | Whether the PR is a draft | PRs only |

## Project Structure

```
triage/
├── main.go                  # Entry point
├── cmd/
│   ├── root.go              # Root command, subcommand registration
│   ├── list.go              # Main list command (also default)
│   ├── cache.go             # Cache management commands
│   ├── config.go            # Config management commands
│   ├── ratelimit.go         # Rate limit status command
│   ├── version.go           # Version command
│   └── options.go           # Shared CLI options
├── config/
│   └── config.go            # Configuration loading and defaults
├── internal/
│   ├── github/
│   │   ├── client.go        # GitHub REST API client, search queries
│   │   ├── graphql.go       # GraphQL batch enrichment
│   │   ├── ratelimit.go     # Global rate limit state tracking
│   │   ├── cache.go         # Multi-tier caching layer
│   │   ├── details.go       # Enrichment orchestration
│   │   ├── notifications.go # Notification fetching
│   │   └── types.go         # Data structures
│   ├── triage/
│   │   ├── engine.go        # Prioritization engine, filters
│   │   ├── heuristics.go    # Scoring logic
│   │   └── types.go         # Priority types
│   ├── tui/
│   │   ├── tui.go           # TUI initialization and runner
│   │   ├── model.go         # Progress display model
│   │   ├── list_model.go    # Interactive list model
│   │   ├── list_view.go     # List rendering
│   │   ├── events.go        # Event types (task, rate limit, etc.)
│   │   ├── task.go          # Task progress tracking
│   │   └── styles.go        # Lipgloss styles
│   ├── output/
│   │   ├── formatter.go     # Formatter interface
│   │   ├── table.go         # Table output
│   │   └── json.go          # JSON output
│   ├── log/
│   │   └── logger.go        # Structured logging
│   └── resolved/
│       └── store.go         # Persistent "done" item tracking
└── docs/
    └── ARCHITECTURE.md      # This file
```

## Concurrency

Data fetching uses parallel goroutines for the four data sources (notifications, review-requested PRs, authored PRs, assigned issues), each with independent caching.

Detail enrichment uses GraphQL batch queries:

- Items are batched into groups of 50 for efficient API usage
- First pass checks cache to avoid unnecessary API calls
- GraphQL allows fetching all fields for multiple items in a single request
- Progress callback updates the TUI during fetching

## GitHub API Usage

The tool uses the [google/go-github](https://github.com/google/go-github) library (v57) for REST APIs and direct HTTP for GraphQL.

### REST API Endpoints (Core + Search)

| Endpoint | API Type | Purpose |
|----------|----------|---------|
| `GET /notifications` | Core | Unread notifications |
| `GET /user` | Core | Authenticated user info |
| `GET /rate_limit` | Core | Rate limit status |
| `GET /search/issues?q=is:pr+is:open+review-requested:{user}` | Search | PRs awaiting review |
| `GET /search/issues?q=is:pr+is:open+author:{user}` | Search | User's open PRs |
| `GET /search/issues?q=is:issue+is:open+assignee:{user}` | Search | Assigned issues |

### GraphQL API

Enrichment uses a single batched GraphQL query per 50 items:

```graphql
query {
  repo0: repository(owner: "owner", name: "repo") {
    pullRequest(number: 123) {
      number, state, additions, deletions, changedFiles,
      isDraft, mergeable, author { login },
      reviews(last: 100) { nodes { state, author { login } } },
      commits(last: 1) { nodes { commit { statusCheckRollup { state } } } }
      # ... more fields
    }
  }
  repo1: repository(...) { ... }
  # ... up to 50 items per query
}
```

This batching reduces API calls from ~300 (3 per item × 100 items) to just 2 queries.

#### API Quota Optimization

Triage uses GitHub's different API quotas efficiently:

| API | Quota | Used For |
|-----|-------|----------|
| **Core API** | 5,000/hour | Fetching notifications |
| **Search API** | 30/minute | Finding review-requested PRs, authored PRs, assigned issues |
| **GraphQL API** | 5,000/hour | Batch enrichment of PR/issue details |

**GraphQL Batching**: Instead of making 3+ REST API calls per item (details, reviews, CI status), triage batches up to 50 items into a single GraphQL query. This reduces enrichment from ~300 Core API calls to just 2 GraphQL calls for 100 items.

This separation means you're unlikely to hit rate limits during normal use, and even if one API is exhausted, others remain available.

#### Graceful Rate Limit Handling

When rate limited, triage handles it gracefully:
- **TUI displays a warning** instead of spamming log messages
- **Returns cached data** when available
- **Skips enrichment** rather than failing completely
