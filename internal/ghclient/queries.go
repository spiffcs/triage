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

// Query templates parsed at init time
var (
	orphanedQueryTemplate  string
	prBatchItemTemplate    *template.Template
	issueBatchItemTemplate *template.Template
)

func init() {
	// Load orphaned query template
	data, err := queryFiles.ReadFile("queries/orphaned.graphql")
	if err != nil {
		panic(fmt.Sprintf("failed to load orphaned.graphql: %v", err))
	}
	orphanedQueryTemplate = string(data)

	// Parse PR batch item template
	prData, err := queryFiles.ReadFile("queries/pr_batch_item.graphql")
	if err != nil {
		panic(fmt.Sprintf("failed to load pr_batch_item.graphql: %v", err))
	}
	prBatchItemTemplate = template.Must(template.New("pr_batch_item").Parse(string(prData)))

	// Parse Issue batch item template
	issueData, err := queryFiles.ReadFile("queries/issue_batch_item.graphql")
	if err != nil {
		panic(fmt.Sprintf("failed to load issue_batch_item.graphql: %v", err))
	}
	issueBatchItemTemplate = template.Must(template.New("issue_batch_item").Parse(string(issueData)))
}

// BuildOrphanedQuery builds the GraphQL query for fetching orphaned contributions.
// Since GitHub's GraphQL API doesn't support variables in the same way as a client library,
// we inline the owner/repo values directly.
func BuildOrphanedQuery(owner, repo string) string {
	// The orphaned.graphql uses GraphQL variable syntax, but we need to inline the values
	// since we're using raw HTTP requests without a GraphQL client library.
	// Convert the parameterized query to an inline query.
	query := strings.Replace(orphanedQueryTemplate, "$owner: String!, $repo: String!", "", 1)
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
func BuildPRBatchQuery(items []BatchItem) (string, error) {
	var sb strings.Builder
	sb.WriteString("query {\n")

	for _, item := range items {
		var buf bytes.Buffer
		if err := prBatchItemTemplate.Execute(&buf, item); err != nil {
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
func BuildIssueBatchQuery(items []BatchItem) (string, error) {
	var sb strings.Builder
	sb.WriteString("query {\n")

	for _, item := range items {
		var buf bytes.Buffer
		if err := issueBatchItemTemplate.Execute(&buf, item); err != nil {
			return "", fmt.Errorf("failed to execute Issue template for %s: %w", item.Alias, err)
		}
		sb.WriteString("  ")
		sb.WriteString(strings.ReplaceAll(buf.String(), "\n", "\n  "))
		sb.WriteString("\n")
	}

	sb.WriteString("}")
	return sb.String(), nil
}
