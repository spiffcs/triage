package ghclient

import (
	"strings"
	"testing"
)

func TestBuildOrphanedQuery(t *testing.T) {
	query := BuildOrphanedQuery("testowner", "testrepo")

	// Verify owner and repo are substituted
	if !strings.Contains(query, `"testowner"`) {
		t.Error("query should contain owner")
	}
	if !strings.Contains(query, `"testrepo"`) {
		t.Error("query should contain repo")
	}

	// Verify required fields are present
	requiredFields := []string{
		"repository(",
		"issues(",
		"pullRequests(",
		"number",
		"title",
		"createdAt",
		"updatedAt",
		"author",
		"authorAssociation",
		"assignees",
		"labels",
		"comments(",
		"reviews(",
	}

	for _, field := range requiredFields {
		if !strings.Contains(query, field) {
			t.Errorf("query should contain %q", field)
		}
	}
}

func TestBuildPRBatchQuery(t *testing.T) {
	items := []BatchItem{
		{Alias: "pr0", Owner: "owner1", Repo: "repo1", Number: 123},
		{Alias: "pr1", Owner: "owner2", Repo: "repo2", Number: 456},
	}

	query, err := BuildPRBatchQuery(items)
	if err != nil {
		t.Fatalf("BuildPRBatchQuery failed: %v", err)
	}

	// Verify query structure
	if !strings.HasPrefix(query, "query {") {
		t.Error("query should start with 'query {'")
	}
	if !strings.HasSuffix(strings.TrimSpace(query), "}") {
		t.Error("query should end with '}'")
	}

	// Verify aliases and values
	if !strings.Contains(query, "pr0: repository(") {
		t.Error("query should contain pr0 alias")
	}
	if !strings.Contains(query, "pr1: repository(") {
		t.Error("query should contain pr1 alias")
	}
	if !strings.Contains(query, `"owner1"`) {
		t.Error("query should contain owner1")
	}
	if !strings.Contains(query, `"repo2"`) {
		t.Error("query should contain repo2")
	}
	if !strings.Contains(query, "number: 123") {
		t.Error("query should contain number 123")
	}
	if !strings.Contains(query, "number: 456") {
		t.Error("query should contain number 456")
	}

	// Verify required PR fields
	requiredFields := []string{
		"pullRequest(",
		"number",
		"state",
		"additions",
		"deletions",
		"changedFiles",
		"isDraft",
		"mergeable",
		"reviewDecision",
		"reviewRequests(",
		"latestReviews(",
		"commits(",
		"statusCheckRollup",
	}

	for _, field := range requiredFields {
		if !strings.Contains(query, field) {
			t.Errorf("query should contain %q", field)
		}
	}
}

func TestBuildIssueBatchQuery(t *testing.T) {
	items := []BatchItem{
		{Alias: "issue0", Owner: "myorg", Repo: "myrepo", Number: 789},
	}

	query, err := BuildIssueBatchQuery(items)
	if err != nil {
		t.Fatalf("BuildIssueBatchQuery failed: %v", err)
	}

	// Verify query structure
	if !strings.HasPrefix(query, "query {") {
		t.Error("query should start with 'query {'")
	}

	// Verify alias and values
	if !strings.Contains(query, "issue0: repository(") {
		t.Error("query should contain issue0 alias")
	}
	if !strings.Contains(query, `"myorg"`) {
		t.Error("query should contain myorg")
	}
	if !strings.Contains(query, `"myrepo"`) {
		t.Error("query should contain myrepo")
	}
	if !strings.Contains(query, "number: 789") {
		t.Error("query should contain number 789")
	}

	// Verify required Issue fields
	requiredFields := []string{
		"issue(",
		"number",
		"state",
		"createdAt",
		"updatedAt",
		"closedAt",
		"author",
		"assignees(",
		"labels(",
		"comments(",
	}

	for _, field := range requiredFields {
		if !strings.Contains(query, field) {
			t.Errorf("query should contain %q", field)
		}
	}
}

func TestBuildPRBatchQueryEmpty(t *testing.T) {
	query, err := BuildPRBatchQuery([]BatchItem{})
	if err != nil {
		t.Fatalf("BuildPRBatchQuery failed: %v", err)
	}

	// Empty batch should still be valid query structure
	if !strings.Contains(query, "query {") {
		t.Error("empty batch should produce valid query structure")
	}
}

func TestBuildIssueBatchQueryEmpty(t *testing.T) {
	query, err := BuildIssueBatchQuery([]BatchItem{})
	if err != nil {
		t.Fatalf("BuildIssueBatchQuery failed: %v", err)
	}

	// Empty batch should still be valid query structure
	if !strings.Contains(query, "query {") {
		t.Error("empty batch should produce valid query structure")
	}
}
