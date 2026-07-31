package readmodel

import "testing"

func TestIsGenerated(t *testing.T) {
	generated := []string{
		"internal/webserver/dist/assets/index-abc.js",
		"web/node_modules/react/index.js",
		"dist/app.js",
		"pkg/vendor/lib/x.go",
		"static/app.min.js",
		"styles.min.css",
	}
	source := []string{
		"internal/graph/store.go",
		"web/src/App.tsx",
		"cmd/codeindex/main.go",
		"editors/vscode/src/core.ts",
	}
	for _, f := range generated {
		if !isGenerated(f) {
			t.Errorf("isGenerated(%q) = false, want true", f)
		}
	}
	for _, f := range source {
		if isGenerated(f) {
			t.Errorf("isGenerated(%q) = true, want false", f)
		}
	}
}
