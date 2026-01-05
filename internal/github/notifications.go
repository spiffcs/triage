package github

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-github/v57/github"
)

// NotificationOptions configures notification fetching
type NotificationOptions struct {
	All           bool          // Include read notifications
	Since         time.Time     // Only notifications updated after this time
	Participating bool          // Only participating notifications
	Repos         []string      // Filter to specific repos (owner/repo format)
	Types         []SubjectType // Filter to specific subject types
}

// ListNotifications fetches notifications with optional filtering
func (c *Client) ListNotifications(opts NotificationOptions) ([]Notification, error) {
	var allNotifications []Notification

	listOpts := &github.NotificationListOptions{
		All:           opts.All,
		Participating: opts.Participating,
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	}

	if !opts.Since.IsZero() {
		listOpts.Since = opts.Since
	}

	for {
		notifications, resp, err := c.client.Activity.ListNotifications(c.ctx, listOpts)
		if err != nil {
			return nil, fmt.Errorf("failed to list notifications: %w", err)
		}

		for _, n := range notifications {
			notification := convertNotification(n)

			// Apply type filter
			if len(opts.Types) > 0 && !containsType(opts.Types, notification.Subject.Type) {
				continue
			}

			// Apply repo filter
			if len(opts.Repos) > 0 && !containsRepo(opts.Repos, notification.Repository.FullName) {
				continue
			}

			allNotifications = append(allNotifications, notification)
		}

		if resp.NextPage == 0 {
			break
		}
		listOpts.Page = resp.NextPage
	}

	return allNotifications, nil
}

// ListUnreadNotifications fetches only unread notifications (convenience method)
func (c *Client) ListUnreadNotifications(since time.Time) ([]Notification, error) {
	return c.ListNotifications(NotificationOptions{
		All:   false,
		Since: since,
		Types: []SubjectType{SubjectIssue, SubjectPullRequest}, // Only issues and PRs
	})
}

// MarkAsRead marks a notification as read
func (c *Client) MarkAsRead(notificationID string) error {
	_, err := c.client.Activity.MarkThreadRead(c.ctx, notificationID)
	if err != nil {
		return fmt.Errorf("failed to mark notification as read: %w", err)
	}
	return nil
}

// convertNotification converts a GitHub API notification to our type
func convertNotification(n *github.Notification) Notification {
	notification := Notification{
		ID:        n.GetID(),
		Reason:    NotificationReason(n.GetReason()),
		Unread:    n.GetUnread(),
		UpdatedAt: n.GetUpdatedAt().Time,
		URL:       n.GetURL(),
	}

	if repo := n.GetRepository(); repo != nil {
		notification.Repository = Repository{
			ID:       repo.GetID(),
			Name:     repo.GetName(),
			FullName: repo.GetFullName(),
			Private:  repo.GetPrivate(),
			HTMLURL:  repo.GetHTMLURL(),
		}
	}

	if subject := n.GetSubject(); subject != nil {
		notification.Subject = Subject{
			Title: subject.GetTitle(),
			URL:   subject.GetURL(),
			Type:  SubjectType(subject.GetType()),
		}
	}

	return notification
}

// ExtractIssueNumber extracts the issue/PR number from the API URL
func ExtractIssueNumber(apiURL string) (int, error) {
	// URL format: https://api.github.com/repos/owner/repo/issues/123
	// or: https://api.github.com/repos/owner/repo/pulls/123
	parts := strings.Split(apiURL, "/")
	if len(parts) < 2 {
		return 0, fmt.Errorf("invalid API URL format: %s", apiURL)
	}

	numStr := parts[len(parts)-1]
	num, err := strconv.Atoi(numStr)
	if err != nil {
		return 0, fmt.Errorf("failed to parse issue number from URL %s: %w", apiURL, err)
	}

	return num, nil
}

func containsType(types []SubjectType, t SubjectType) bool {
	for _, typ := range types {
		if typ == t {
			return true
		}
	}
	return false
}

func containsRepo(repos []string, fullName string) bool {
	for _, repo := range repos {
		if strings.EqualFold(repo, fullName) {
			return true
		}
	}
	return false
}
