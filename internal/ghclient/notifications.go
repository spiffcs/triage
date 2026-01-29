package ghclient

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	gh "github.com/google/go-github/v57/github"
	"github.com/spiffcs/triage/internal/model"
)

// NotificationOptions configures notification fetching
type NotificationOptions struct {
	All           bool                // Include read notifications
	Since         time.Time           // Only notifications updated after this time
	Participating bool                // Only participating notifications
	Repos         []string            // Filter to specific repos (owner/repo format)
	Types         []model.SubjectType // Filter to specific subject types
}

// ListNotifications fetches notifications with optional filtering.
// Uses parallel fetching when multiple pages are detected.
func (c *Client) ListNotifications(ctx context.Context, opts NotificationOptions) ([]model.Item, error) {
	listOpts := &gh.NotificationListOptions{
		All:           opts.All,
		Participating: opts.Participating,
		ListOptions: gh.ListOptions{
			PerPage: 100,
		},
	}

	if !opts.Since.IsZero() {
		listOpts.Since = opts.Since
	}

	// Fetch first page to get pagination info
	notifications, resp, err := c.client.Activity.ListNotifications(ctx, listOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to list notifications: %w", err)
	}

	// Convert first page results
	allItems := filterNotifications(notifications, opts)

	// If no more pages, return early
	if resp.NextPage == 0 {
		return allItems, nil
	}

	// Determine last page from response
	lastPage := resp.LastPage
	if lastPage == 0 {
		// Fallback to sequential if we can't determine last page
		return c.listNotificationsSequential(ctx, opts, allItems, resp.NextPage)
	}

	// Fetch remaining pages in parallel
	type pageResult struct {
		page  int
		items []model.Item
		err   error
	}

	numPages := lastPage - 1 // pages 2 through lastPage
	results := make(chan pageResult, numPages)

	var wg sync.WaitGroup
	for page := 2; page <= lastPage; page++ {
		wg.Add(1)
		go func(p int) {
			defer wg.Done()
			pageOpts := &gh.NotificationListOptions{
				All:           opts.All,
				Participating: opts.Participating,
				ListOptions: gh.ListOptions{
					PerPage: 100,
					Page:    p,
				},
			}
			if !opts.Since.IsZero() {
				pageOpts.Since = opts.Since
			}

			notifs, _, err := c.client.Activity.ListNotifications(ctx, pageOpts)
			if err != nil {
				results <- pageResult{page: p, err: err}
				return
			}
			results <- pageResult{page: p, items: filterNotifications(notifs, opts)}
		}(page)
	}

	// Close results channel when all goroutines complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	pageResults := make([]pageResult, 0, numPages)
	for result := range results {
		if result.err != nil {
			return nil, fmt.Errorf("failed to list items page %d: %w", result.page, result.err)
		}
		pageResults = append(pageResults, result)
	}

	// Combine all results (order doesn't matter for items)
	for _, pr := range pageResults {
		allItems = append(allItems, pr.items...)
	}

	return allItems, nil
}

// filterNotifications applies type and repo filters to raw notifications
func filterNotifications(notifications []*gh.Notification, opts NotificationOptions) []model.Item {
	var result []model.Item
	for _, n := range notifications {
		item := convertNotification(n)

		// Apply type filter
		if len(opts.Types) > 0 && !containsType(opts.Types, item.Subject.Type) {
			continue
		}

		// Apply repo filter
		if len(opts.Repos) > 0 && !containsRepo(opts.Repos, item.Repository.FullName) {
			continue
		}

		result = append(result, item)
	}
	return result
}

// listNotificationsSequential fetches remaining pages sequentially (fallback)
func (c *Client) listNotificationsSequential(ctx context.Context, opts NotificationOptions, existing []model.Item, startPage int) ([]model.Item, error) {
	allItems := existing

	listOpts := &gh.NotificationListOptions{
		All:           opts.All,
		Participating: opts.Participating,
		ListOptions: gh.ListOptions{
			PerPage: 100,
			Page:    startPage,
		},
	}

	if !opts.Since.IsZero() {
		listOpts.Since = opts.Since
	}

	for {
		notifications, resp, err := c.client.Activity.ListNotifications(ctx, listOpts)
		if err != nil {
			return nil, fmt.Errorf("failed to list items: %w", err)
		}

		allItems = append(allItems, filterNotifications(notifications, opts)...)

		if resp.NextPage == 0 {
			break
		}
		listOpts.Page = resp.NextPage
	}

	return allItems, nil
}

// ListUnreadNotifications fetches only unread notifications (convenience method)
func (c *Client) ListUnreadNotifications(ctx context.Context, since time.Time) ([]model.Item, error) {
	return c.ListNotifications(ctx, NotificationOptions{
		All:   false,
		Since: since,
		Types: []model.SubjectType{model.SubjectIssue, model.SubjectPullRequest}, // Only issues and PRs
	})
}

// MarkAsRead marks a notification as read
func (c *Client) MarkAsRead(ctx context.Context, notificationID string) error {
	_, err := c.client.Activity.MarkThreadRead(ctx, notificationID)
	if err != nil {
		return fmt.Errorf("failed to mark notification as read: %w", err)
	}
	return nil
}

// convertNotification converts a GitHub API notification to our model type
func convertNotification(n *gh.Notification) model.Item {
	item := model.Item{
		ID:        n.GetID(),
		Reason:    model.ItemReason(n.GetReason()),
		Unread:    n.GetUnread(),
		UpdatedAt: n.GetUpdatedAt().Time,
		URL:       n.GetURL(),
	}

	if repo := n.GetRepository(); repo != nil {
		item.Repository = model.Repository{
			ID:       repo.GetID(),
			Name:     repo.GetName(),
			FullName: repo.GetFullName(),
			Private:  repo.GetPrivate(),
			HTMLURL:  repo.GetHTMLURL(),
		}
	}

	if subject := n.GetSubject(); subject != nil {
		item.Subject = model.Subject{
			Title: subject.GetTitle(),
			URL:   subject.GetURL(),
			Type:  model.SubjectType(subject.GetType()),
		}
	}

	return item
}

func containsType(types []model.SubjectType, t model.SubjectType) bool {
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
