# github-prio

A CLI tool that analyzes your GitHub notifications to help you prioritize work. It fetches unread notifications, enriches them with issue/PR details, and ranks them using heuristics. Optionally uses Claude AI for deeper analysis.

## Installation

```bash
go install github.com/hal/github-prio/cmd/github-prio@latest
```

Or build from source:

```bash
git clone https://github.com/hal/github-prio.git
cd github-prio
go build -o github-prio ./cmd/github-prio
```

## Setup

### GitHub Token

Create a [Personal Access Token](https://github.com/settings/tokens) with the following scopes:
- `notifications` - Read notifications
- `repo` - Access private repo details (optional, for private repos)

Set the token:

```bash
# Via environment variable
export GITHUB_TOKEN=ghp_xxxxxxxxxxxx

# Or save to config
github-prio config set token ghp_xxxxxxxxxxxx
```

### Claude API Key (Optional)

For AI-powered analysis, set your Anthropic API key:

```bash
export ANTHROPIC_API_KEY=sk-ant-xxxxxxxxxxxx

# Or save to config
github-prio config set claude-key sk-ant-xxxxxxxxxxxx
```

## Usage

### List Notifications

```bash
# List prioritized notifications (default: last 6 months)
github-prio list

# Limit time range
github-prio list --since 1w      # Last week
github-prio list --since 30d     # Last 30 days

# Quick mode - skip fetching details (faster but less accurate)
github-prio list -q

# Filter by category
github-prio list -c urgent
github-prio list -c important
github-prio list -c low-hanging
github-prio list -c fyi

# Filter by notification reason
github-prio list -r mention
github-prio list -r review_requested
github-prio list -r author

# Include merged/closed items (hidden by default)
github-prio list --include-merged
github-prio list --include-closed

# Output formats
github-prio list -f table      # Default
github-prio list -f json       # JSON for scripting
github-prio list -f markdown   # Markdown for notes

# Limit results
github-prio list -l 20         # Top 20 only

# With AI analysis
github-prio list -a            # Adds AI insights
github-prio list -a -v         # Verbose with full analysis
```

### Summary

```bash
github-prio summary
```

### AI Analysis

```bash
# Analyze top 5 notifications with Claude
github-prio analyze
```

### Cache Management

Details are cached for 24 hours to speed up subsequent runs:

```bash
github-prio cache stats    # Show cache statistics
github-prio cache clear    # Clear the cache
```

### Configuration

```bash
github-prio config show                    # Show current config
github-prio config set token <TOKEN>       # Set GitHub token
github-prio config set claude-key <KEY>    # Set Claude API key
github-prio config set format json         # Set default output format
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

2. **Use quick mode for triage** - `github-prio list -q` skips detail fetching for a fast overview.

3. **Focus on urgent items** - `github-prio list -c urgent -l 10` shows your top 10 urgent items.

4. **Clear old notifications** - Use `github-prio mark-read <id>` or mark them as read on GitHub to reduce noise.

5. **Adjust concurrency** - If you hit rate limits, reduce workers: `github-prio list -w 5`

## Configuration File

Config is stored at `~/.config/github-prio/config.yaml`:

```yaml
github_token: ghp_xxxxxxxxxxxx
claude_api_key: sk-ant-xxxxxxxxxxxx
default_format: table
exclude_repos:
  - some-org/noisy-repo
```

## Cache Location

Notification details are cached at `~/.cache/github-prio/details/` for 24 hours.

## License

MIT
