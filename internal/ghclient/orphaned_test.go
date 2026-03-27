package ghclient

import (
	"testing"
	"time"
)

func TestAnalyzeComments_SkipsBots(t *testing.T) {
	now := time.Now()
	comments := []commentNode{
		{Author: &actorRef{Login: "contributor"}, AuthorAssociation: "NONE", CreatedAt: now.Add(-3 * time.Hour)},
		{Author: &actorRef{Login: "codecov[bot]"}, AuthorAssociation: "NONE", CreatedAt: now.Add(-2 * time.Hour)},
		{Author: &actorRef{Login: "contributor"}, AuthorAssociation: "NONE", CreatedAt: now.Add(-1 * time.Hour)},
	}

	// Without bot skipping, the consecutive count would be 1 (only the last comment).
	// With bot skipping, codecov[bot] is ignored so both contributor comments are consecutive.
	_, consecutive := analyzeComments(comments, "contributor")
	if consecutive != 2 {
		t.Errorf("analyzeComments() consecutive = %d, want 2 (bot should be skipped)", consecutive)
	}
}

func TestAnalyzeComments_BotDoesNotCountAsTeam(t *testing.T) {
	now := time.Now()
	comments := []commentNode{
		{Author: &actorRef{Login: "contributor"}, AuthorAssociation: "NONE", CreatedAt: now.Add(-2 * time.Hour)},
		{Author: &actorRef{Login: "dependabot[bot]"}, AuthorAssociation: "NONE", CreatedAt: now.Add(-1 * time.Hour)},
	}

	lastTeam, _ := analyzeComments(comments, "contributor")
	if lastTeam != nil {
		t.Error("bot comment should not count as team activity")
	}
}

func TestAnalyzeComments_TeamBreaksConsecutive(t *testing.T) {
	now := time.Now()
	comments := []commentNode{
		{Author: &actorRef{Login: "contributor"}, AuthorAssociation: "NONE", CreatedAt: now.Add(-3 * time.Hour)},
		{Author: &actorRef{Login: "maintainer"}, AuthorAssociation: "MEMBER", CreatedAt: now.Add(-2 * time.Hour)},
		{Author: &actorRef{Login: "contributor"}, AuthorAssociation: "NONE", CreatedAt: now.Add(-1 * time.Hour)},
	}

	lastTeam, consecutive := analyzeComments(comments, "contributor")
	if consecutive != 1 {
		t.Errorf("analyzeComments() consecutive = %d, want 1 (team comment should break streak)", consecutive)
	}
	if lastTeam == nil {
		t.Error("expected team activity to be tracked")
	}
}

func TestAnalyzeComments_AllBots(t *testing.T) {
	now := time.Now()
	comments := []commentNode{
		{Author: &actorRef{Login: "contributor"}, AuthorAssociation: "NONE", CreatedAt: now.Add(-3 * time.Hour)},
		{Author: &actorRef{Login: "github-actions[bot]"}, AuthorAssociation: "NONE", CreatedAt: now.Add(-2 * time.Hour)},
		{Author: &actorRef{Login: "codecov[bot]"}, AuthorAssociation: "NONE", CreatedAt: now.Add(-1 * time.Hour)},
	}

	_, consecutive := analyzeComments(comments, "contributor")
	if consecutive != 1 {
		t.Errorf("analyzeComments() consecutive = %d, want 1", consecutive)
	}
}

func TestDefaultConsecutiveAuthorComments(t *testing.T) {
	if defaultConsecutiveAuthorComments != 3 {
		t.Errorf("defaultConsecutiveAuthorComments = %d, want 3", defaultConsecutiveAuthorComments)
	}
}
