# priority

A CLI tool that analyzes your GitHub notifications to help you prioritize work. It aggregates data from multiple sources—unread notifications, PRs awaiting your review, and your own open PRs—enriches them with details, and ranks them using heuristics. Optionally uses Claude AI for deeper analysis.

![Demo](.github/demo.png)

## Installation

```bash
go install github.com/spiffcs/priority/cmd/priority@latest
```

Or build from source:

```bash
git clone https://github.com/spiffcs/priority.git
cd priority
go build -o priority ./cmd/priority
```

## Setup

### Github token
```
GITHUB_TOKEN=(gh auth token) priority list
```

## Usage

### List Notifications

```bash
# List prioritized notifications (default: last 6 months)
priority list

# Limit time range (default: 6mo)
priority list --since 30m     # Last 30 minutes
priority list --since 2h      # Last 2 hours
priority list --since 1d      # Last day
priority list --since 1w      # Last week
priority list --since 30d     # Last 30 days
priority list --since 6mo     # Last 6 months
priority list --since 1y      # Last year

# Supported time units:
#   Minutes: m, min, mins
#   Hours:   h, hr, hrs, hour, hours
#   Days:    d, day, days
#   Weeks:   w, wk, wks, week, weeks
#   Months:  mo, month, months (30 days)
#   Years:   y, yr, yrs, year, years (365 days)

# Quick mode - skip fetching details (faster but less accurate)
priority list -q

# Filter by category
priority list -c urgent
priority list -c important
priority list -c low-hanging
priority list -c fyi

# Filter by notification reason
priority list -r mention
priority list -r review_requested
priority list -r author

# Filter by type
priority list -t pr          # Show only pull requests
priority list -t issue       # Show only issues

# Filter by repository
priority list --repo owner/repo

# Include merged/closed items (hidden by default)
priority list --include-merged
priority list --include-closed

# Output formats
priority list -f table      # Default
priority list -f json       # JSON for scripting
priority list -f markdown   # Markdown for notes

# Limit results
priority list -l 20         # Top 20 only

# With AI analysis
priority list -a            # Adds AI insights
priority list -a -v         # Verbose with full analysis
```

### Summary

```bash
priority summary
```

### AI Analysis

```bash
# Analyze top 5 notifications with Claude
priority analyze
```

### Cache Management

The tool uses a two-tier caching strategy:
- **Item details** (issue/PR metadata): cached for 24 hours
- **PR lists** (review-requested and authored PRs): cached for 5 minutes

```bash
priority cache stats    # Show cache statistics
priority cache clear    # Clear all caches
```

### Configuration

```bash
priority config show                    # Show current config
priority config set format json         # Set default output format
```

**Note:** GitHub tokens must be set via the `GITHUB_TOKEN` environment variable (not stored in config files) following [12-factor app](https://12factor.net/config) security best practices.

## Priority Scoring

Notifications are scored based on multiple factors to determine priority.

### Base Scores by Notification Reason

| Reason | Score | Description |
|--------|-------|-------------|
| `review_requested` | 100 | Someone requested your review |
| `mention` | 90 | You were directly @mentioned |
| `team_mention` | 85 | Your team was @mentioned |
| `author` | 70 | Activity on an issue/PR you created |
| `assign` | 60 | You were assigned to the issue |
| `comment` | 30 | New comment on a thread you're watching |
| `state_change` | 25 | Issue/PR was opened, closed, or merged |
| `subscribed` | 10 | Activity on a repo you're watching |
| `ci_activity` | 5 | CI/CD activity |

### Score Modifiers

| Modifier | Score | Condition |
|----------|-------|-----------|
| Open state | +10 | Issue/PR is still open |
| Closed state | -30 | Issue/PR was closed/merged |
| Hot topic | +15 | More than 10 comments |
| Low-hanging fruit | +20 | Small PR or has "good first issue" label |
| Age bonus | +2/day | Older unread items (capped at +30) |
| Needs update | +20 | Your PR has "changes requested" |

### Priority Levels

| Level | Score Range | Display |
|-------|-------------|---------|
| URGENT | 90+ | Red |
| HIGH | 60-89 | Yellow |
| MEDIUM | 30-59 | Cyan |
| LOW | 0-29 | White |

### Categories

- **Urgent**: Review requests and direct mentions
- **Important**: Your authored PRs needing attention, assignments
- **Quick Win**: Small PRs, items labeled "good first issue", "help wanted", etc.
- **FYI**: Subscribed notifications, general activity

### Low-Hanging Fruit Detection

Items are marked as "Quick Win" if they match any of these criteria:

- Labels containing: `good first issue`, `help wanted`, `easy`, `beginner`, `trivial`, `documentation`, `docs`, `typo`
- Pull requests with ≤3 files changed AND ≤50 lines changed

## Output Example

```
Priority  Category      Repository                      Title                                               Reason              Age
------------------------------------------------------------------------------------------------------------------------
URGENT    Urgent        anchore/syft                    Fix SBOM parsing for container images               review_requested    3d
URGENT    Urgent        my-org/api                      Production crash on auth endpoint                   mention             1w
HIGH      Important     golang/go                       Add new feature for error handling                  author              2d
HIGH      Quick Win     some/repo                       Fix typo in README                                  subscribed          1d
MEDIUM    FYI           other/repo                      Discussion on API design                            comment             5d

2 urgent items need your attention.
```

## Tips

1. **First run is slow** - Fetching details for many notifications takes time. Subsequent runs use the cache.
2. **Use quick mode for triage** - `priority list -q` skips detail fetching for a fast overview.
3. **Focus on urgent items** - `priority list -c urgent -l 10` shows your top 10 urgent items.
4. **Adjust concurrency** - If you hit rate limits, reduce workers: `priority list -w 5`

## Configuration File

Config is stored at `~/.config/priority/config.yaml`:

```yaml
default_format: table
exclude_repos:
  - some-org/noisy-repo
```

## Cache Location

Cached data is stored at `~/.cache/priority/details/`.

## Architecture

See [ARCHITECTURE.md](ARCHITECTURE.md) for details on data flow, caching strategy, and project structure.

## License

MIT
