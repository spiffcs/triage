# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| latest  | :white_check_mark: |

## Reporting a Vulnerability

We take security vulnerabilities seriously. If you discover a security issue, please report it responsibly.

**Please do NOT report security vulnerabilities through public GitHub issues.**

Instead, please report them via GitHub's private vulnerability reporting:

1. Go to the [Security tab](../../security) of this repository
2. Click "Report a vulnerability"
3. Fill out the form with details about the vulnerability

### What to include

- Type of issue (e.g., buffer overflow, SQL injection, cross-site scripting, etc.)
- Full paths of source file(s) related to the issue
- Location of the affected source code (tag/branch/commit or direct URL)
- Any special configuration required to reproduce the issue
- Step-by-step instructions to reproduce the issue
- Proof-of-concept or exploit code (if possible)
- Impact of the issue, including how an attacker might exploit it

### Response Timeline

- We will acknowledge receipt of your vulnerability report within 3 business days
- We will provide a more detailed response within 10 business days
- We will work with you to understand and resolve the issue promptly

## Security Best Practices for Users

- Always use the latest version of triage
- Never commit your GitHub token to version control
- Use `gh auth login` and `GITHUB_TOKEN=$(gh auth token) triage` to avoid long-lived tokens in shell profiles — `gh` stores credentials in your OS keychain
- If you must create a classic token manually, set an expiration and use the minimum scopes: `notifications` and `repo`
- triage only reads data — it never writes, comments, or modifies anything. A classic token is required because GitHub's Notifications API does not support fine-grained tokens
