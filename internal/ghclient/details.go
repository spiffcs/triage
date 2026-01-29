package ghclient

// EnrichmentProgress tracks enrichment progress.
// Deprecated: Use Enricher.Enrich instead.
type EnrichmentProgress struct {
	Total     int
	Completed int64
	Errors    int64
}
