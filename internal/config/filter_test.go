package config

import "testing"

func TestFilterDefaults(t *testing.T) {
	f := NewFilter(Config{})

	skipDirs := [][2]string{
		{"internal/webserver/dist", "dist"},
		{"web/node_modules", "node_modules"},
		{"pkg/vendor", "vendor"},
		{".git", ".git"},
	}
	for _, d := range skipDirs {
		if !f.SkipDir(d[0], d[1]) {
			t.Errorf("SkipDir(%q,%q) = false, want true", d[0], d[1])
		}
	}
	if f.SkipDir("internal/graph", "graph") {
		t.Error("SkipDir(internal/graph) = true, want false")
	}

	if !f.SkipFile("static/app.min.js") {
		t.Error("app.min.js should be skipped")
	}
	if f.SkipFile("internal/graph/store.go") {
		t.Error("store.go should not be skipped")
	}
}

func TestFilterUserExclude(t *testing.T) {
	f := NewFilter(Config{Exclude: []string{"docs/generated", "**/*.gen.go"}})

	if !f.SkipFile("docs/generated/api.md") {
		t.Error("prefix exclude docs/generated should skip nested file")
	}
	if !f.SkipFile("internal/pb/user.gen.go") {
		t.Error("glob **/*.gen.go should skip user.gen.go")
	}
	if f.SkipFile("internal/pb/user.go") {
		t.Error("user.go should not be skipped")
	}
}

func TestFilterIncludeOverrides(t *testing.T) {
	f := NewFilter(Config{Include: []string{"internal/webserver/dist/keep.js"}})

	// The included file survives the default dist exclusion...
	if f.SkipFile("internal/webserver/dist/keep.js") {
		t.Error("included keep.js should not be skipped")
	}
	// ...its directory is descended (not pruned) so the walk can reach it...
	if f.SkipDir("internal/webserver/dist", "dist") {
		t.Error("dist must be descended when an Include is under it")
	}
	// ...but its non-included siblings are still skipped.
	if !f.SkipFile("internal/webserver/dist/bundle.js") {
		t.Error("non-included dist sibling should still be skipped")
	}
	// An unrelated default dir stays pruned.
	if !f.SkipDir("web/node_modules", "node_modules") {
		t.Error("unrelated node_modules should still be pruned")
	}
}
