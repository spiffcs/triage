package cache

import (
	"testing"

	"github.com/spiffcs/triage/internal/model"
)

func TestCacheKeyStringNoCollision(t *testing.T) {
	c := &Cache{dir: "/tmp/test"}

	// These two keys must produce different filenames.
	// With "/" → "_" replacement they would both be "a_b_c_PullRequest_1.json".
	keyA := Key{RepoFullName: "a/b_c", SubjectType: model.SubjectPullRequest, Number: 1}
	keyB := Key{RepoFullName: "a_b/c", SubjectType: model.SubjectPullRequest, Number: 1}

	if c.cacheKeyString(keyA) == c.cacheKeyString(keyB) {
		t.Errorf("cache keys must not collide:\n  keyA=%q\n  keyB=%q\n  both=%q",
			keyA.RepoFullName, keyB.RepoFullName, c.cacheKeyString(keyA))
	}
}

func TestCacheKeyStringDeterministic(t *testing.T) {
	c := &Cache{dir: "/tmp/test"}

	key := Key{RepoFullName: "owner/repo", SubjectType: model.SubjectIssue, Number: 42}
	a := c.cacheKeyString(key)
	b := c.cacheKeyString(key)

	if a != b {
		t.Errorf("cache key not deterministic: %q != %q", a, b)
	}
}

func TestCacheKeyStringFormat(t *testing.T) {
	c := &Cache{dir: "/tmp/test"}

	key := Key{RepoFullName: "owner/repo", SubjectType: model.SubjectPullRequest, Number: 7}
	got := c.cacheKeyString(key)
	want := "owner~repo_PullRequest_7.json"

	if got != want {
		t.Errorf("cacheKeyString = %q, want %q", got, want)
	}
}
