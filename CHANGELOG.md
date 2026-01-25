# Changelog

## v0.1.0

**[(Full Changelog)](https://github.com/spiffcs/triage/commits/v0.1.0)**

### Features

- Add interactive TUI table interface
- Add TUI and performance updates
- Add PR list caching, `--type` filter, and improve messaging
- Add links to issues in table
- Add review-requested PRs and authors PRs to priority list
- Enhance table output with review state and PR size
- Update priority levels to be Urgent, Important, Quick Win, FYI
- Allow weights and labels to be configurable
- Update cache for all notifications and items
- Add hot topic comment modifier with config
- Add link scoring and new priority level
- Flatten config structure
- Update config command

### Bug Fixes

- Fix presentation for titles with emoji
- Fix colorized truncated output
- Change time threshold from weeks < 4 to days < 30
- Update threshold for better categorization

### Performance

- Small parallel processing improvements

### Tests

- Add basic tests to protect against regression

### Ops

- Update CI with sign and release workflow
- Add dependabot configuration
