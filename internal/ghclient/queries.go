package ghclient

import (
	"bytes"
	"embed"
	"fmt"
	"strings"
	"text/template"
)

//go:embed queries/*.graphql
var queryFiles embed.FS

// queries holds parsed GraphQL query templates.
type queries struct {
	orphanedTemplate string
	prBatchTemplate  *template.Template
	issBatchTemplate *template.Template
}

// loadQueries reads embedded GraphQL files and parses templates.
func loadQueries() (*queries, error) {
	data, err := queryFiles.ReadFile("queries/orphaned.graphql")
	if err != nil {
		return nil, fmt.Errorf("loading orphaned.graphql: %w", err)
	}

	prData, err := queryFiles.ReadFile("queries/pr_batch_item.graphql")
	if err != nil {
		return nil, fmt.Errorf("loading pr_batch_item.graphql: %w", err)
	}
	prTmpl, err := template.New("pr_batch_item").Parse(string(prData))
	if err != nil {
		return nil, fmt.Errorf("parsing pr_batch_item.graphql: %w", err)
	}

	issueData, err := queryFiles.ReadFile("queries/issue_batch_item.graphql")
	if err != nil {
		return nil, fmt.Errorf("loading issue_batch_item.graphql: %w", err)
	}
	issTmpl, err := template.New("issue_batch_item").Parse(string(issueData))
	if err != nil {
		return nil, fmt.Errorf("parsing issue_batch_item.graphql: %w", err)
	}

	return &queries{
		orphanedTemplate: string(data),
		prBatchTemplate:  prTmpl,
		issBatchTemplate: issTmpl,
	}, nil
}

// BuildOrphanedQuery builds the GraphQL query for fetching orphaned contributions.
// Since GitHub's GraphQL API doesn't support variables in the same way as a client library,
// we inline the owner/repo values directly.
func (q *queries) BuildOrphanedQuery(owner, repo string) string {
	query := strings.Replace(q.orphanedTemplate, "$owner: String!, $repo: String!", "", 1)
	query = strings.ReplaceAll(query, "$owner", fmt.Sprintf(`"%s"`, owner))
	query = strings.ReplaceAll(query, "$repo", fmt.Sprintf(`"%s"`, repo))
	query = strings.Replace(query, "query OrphanedContributions()", "query", 1)
	return query
}

// BatchItem represents the parameters for a single item in a batch query.
type BatchItem struct {
	Alias  string
	Owner  string
	Repo   string
	Number int
}

// BuildPRBatchQuery builds a GraphQL query for multiple PRs using aliases.
func (q *queries) BuildPRBatchQuery(items []BatchItem) (string, error) {
	var sb strings.Builder
	sb.WriteString("query {\n")

	for _, item := range items {
		var buf bytes.Buffer
		if err := q.prBatchTemplate.Execute(&buf, item); err != nil {
			return "", fmt.Errorf("failed to execute PR template for %s: %w", item.Alias, err)
		}
		sb.WriteString("  ")
		sb.WriteString(strings.ReplaceAll(buf.String(), "\n", "\n  "))
		sb.WriteString("\n")
	}

	sb.WriteString("}")
	return sb.String(), nil
}

// BuildIssueBatchQuery builds a GraphQL query for multiple Issues using aliases.
func (q *queries) BuildIssueBatchQuery(items []BatchItem) (string, error) {
	var sb strings.Builder
	sb.WriteString("query {\n")

	for _, item := range items {
		var buf bytes.Buffer
		if err := q.issBatchTemplate.Execute(&buf, item); err != nil {
			return "", fmt.Errorf("failed to execute Issue template for %s: %w", item.Alias, err)
		}
		sb.WriteString("  ")
		sb.WriteString(strings.ReplaceAll(buf.String(), "\n", "\n  "))
		sb.WriteString("\n")
	}

	sb.WriteString("}")
	return sb.String(), nil
}
