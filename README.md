# triage

A CLI tool that organizes GitHub notifications, issues, and PRs to help you triage work. It aggregates data from multiple sources including unread notifications, PRs awaiting your review, your own open PRs, and issues assigned to the user running the program. It enriches these items with details and ranks them using configurable heuristics. All titles and repositories are clickable and take the user to the issue, pr, or repository home page.

![Demo](.github/demo.png)

## Installation

```bash
go install github.com/spiffcs/triage/cmd/triage@latest
```

Or build from source:

```bash
git clone https://github.com/spiffcs/triage.git
cd triage
go build -o triage ./cmd/triage
```

## Setup

### Github token
```
# defaults to --since 1w
GITHUB_TOKEN=$(gh auth token) triage
```

## Usage

### List Items

```bash
# Make sure GITHUB_TOKEN is set or frontloaded as seen above
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

# Filter by priority
triage -p urgent
triage -p important
triage -p quick-win
triage -p fyi

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
triage -l 20         # limit the list
```

### Cache Management

The tool uses a three-tier caching strategy to reduce API usage:
- **Item details** (issue/PR metadata): cached for 24 hours
- **Notification lists**: cached for 1 hour
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

### Priority

- **Urgent**: Review requests and direct mentions
- **Important**: Your authored PRs needing attention, assignments
- **Quick Win**: Small PRs, items labeled "good first issue", "help wanted", etc.
- **FYI**: Subscribed notifications, general activity

### Quick Win Detection

Items are marked as "Quick Win" if they match any of these criteria:

- Labels containing: `good first issue`, `help wanted`, `easy`, `beginner`, `trivial`, `documentation`, `docs`, `typo` (configurable via `quick_win_labels`)
- Pull requests with ≤3 files changed AND ≤50 lines changed

## Configuration File

Config is stored at `~/.config/triage/config.yaml`:

```yaml
default_format: table
exclude_repos:
  - some-org/noisy-repo
```

### Customizing Score Weights

You can override the default scoring weights in your config file. Any values not specified will use the defaults shown above.

```yaml
weights:
  base_scores:
    review_requested: 120    # Boost review requests
    mention: 95
    team_mention: 85
    author: 70
    assign: 60
    comment: 30
    state_change: 25
    subscribed: 10
    ci_activity: 5
  modifiers:
    old_unread_bonus: 2      # Per day
    hot_topic_bonus: 15
    low_hanging_bonus: 20
    open_state_bonus: 10
    closed_state_penalty: -50  # Penalize closed items more
    fyi_promotion_threshold: 65
```

Only specify the weights you want to change:

```yaml
weights:
  base_scores:
    review_requested: 200    # Really prioritize reviews
  modifiers:
    closed_state_penalty: -100  # Heavily penalize closed items
```

### Customizing Quick Win Labels

By default, items with these label patterns are marked as "Quick Win":
- `good first issue`, `help wanted`
- `easy`, `beginner`, `trivial`
- `documentation`, `docs`, `typo`

You can override these with your own labels:

```yaml
quick_win_labels:
  - good first issue
  - help wanted
  - low-hanging-fruit
  - quick-fix
  - starter
```

Labels are matched case-insensitively, use substring matching (e.g., `doc` matches `documentation`), and treat hyphens and spaces as equivalent (e.g., `good first issue` matches `good-first-issue`).

## Cache Location

Cached data is stored at `~/.cache/triage/details/`.

## License

Apache-2.0
