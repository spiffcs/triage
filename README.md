# triage

A CLI tool that analyzes your GitHub notifications to help you triage work. It aggregates data from multiple sources—unread notifications, PRs awaiting your review, and your own open PRs—enriches them with details, and ranks them using configurable heuristics.

![Demo](.github/demo.png)

## Installation

```bash
go install github.com/hal/triage/cmd/triage@latest
```

Or build from source:

```bash
git clone https://github.com/hal/triage.git
cd triage
go build -o triage ./cmd/triage
```

## Setup

### Github token
```
GITHUB_TOKEN=$(gh auth token) triage
```

## Usage

### List Notifications

```bash
# List prioritized notifications (default: last 1 week)
triage

# Limit time range (default: 1w)
triage --since 30m     # Last 30 minutes
triage --since 2h      # Last 2 hours
triage --since 1d      # Last day
triage --since 1w      # Last week
triage --since 30d     # Last 30 days
triage --since 6mo     # Last 6 months
triage --since 1y      # Last year

# Supported time units:
#   Minutes: m, min, mins
#   Hours:   h, hr, hrs, hour, hours
#   Days:    d, day, days
#   Weeks:   w, wk, wks, week, weeks
#   Months:  mo, month, months (30 days)
#   Years:   y, yr, yrs, year, years (365 days)

# Filter by category
triage -c urgent
triage -c important
triage -c low-hanging
triage -c fyi

# Filter by notification reason
triage -r mention
triage -r review_requested
triage -r author

# Filter by type
triage -t pr          # Show only pull requests
triage -t issue       # Show only issues

# Filter by repository
triage --repo owner/repo

# Include merged/closed items (hidden by default)
triage --include-merged
triage --include-closed

# Output formats
triage -f table      # Default
triage -f json       # JSON for scripting

# Limit results
triage -l 20         # Top 20 only

```
### Summary

```bash
triage summary
```

### Cache Management

The tool uses a two-tier caching strategy:
- **Item details** (issue/PR metadata): cached for 24 hours
- **PR lists** (review-requested and authored PRs): cached for 5 minutes

```bash
triage cache stats    # Show cache statistics
triage cache clear    # Clear all caches
```

### Configuration

```bash
triage config show                    # Show current config
triage config set format json         # Set default output format
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
2. **Use quick mode for triage** - `triage list -q` skips detail fetching for a fast overview.
3. **Focus on urgent items** - `triage list -c urgent -l 10` shows your top 10 urgent items.
4. **Adjust concurrency** - If you hit rate limits, reduce workers: `triage list -w 5`

## Configuration File

Config is stored at `~/.config/triage/config.yaml`:

```yaml
default_format: table
exclude_repos:
  - some-org/noisy-repo
```

## Cache Location

Cached data is stored at `~/.cache/triage/details/`.

## Architecture

See [ARCHITECTURE.md](ARCHITECTURE.md) for details on data flow, caching strategy, and project structure.

## License

MIT
