//go:build !nollama

package embed

import (
	"strings"
	"testing"
)

func TestProviderSelection(t *testing.T) {
	t.Run("default is local bundled", func(t *testing.T) {
		p, note, err := New(Config{})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		defer p.Close()
		if note != "" {
			t.Fatalf("unexpected note %q", note)
		}
		if !strings.HasPrefix(p.ID(), BundledModelName) {
			t.Fatalf("ID = %q, want bundled", p.ID())
		}
	})

	t.Run("api without credential falls back to local with note", func(t *testing.T) {
		t.Setenv("CODEINDEX_TEST_EMBED_KEY", "")
		p, note, err := New(Config{Provider: "api", APIKeyEnv: "CODEINDEX_TEST_EMBED_KEY"})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		defer p.Close()
		if note == "" {
			t.Fatal("want fallback note")
		}
		if !strings.HasPrefix(p.ID(), BundledModelName) {
			t.Fatalf("ID = %q, want bundled fallback", p.ID())
		}
	})

	t.Run("api with credential selected", func(t *testing.T) {
		t.Setenv("CODEINDEX_TEST_EMBED_KEY", "k")
		p, note, err := New(Config{Provider: "api", APIKeyEnv: "CODEINDEX_TEST_EMBED_KEY", Model: "m"})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		defer p.Close()
		if note != "" || p.ID() != "api/m" {
			t.Fatalf("got note=%q id=%q", note, p.ID())
		}
	})

	t.Run("unknown provider errors", func(t *testing.T) {
		if _, _, err := New(Config{Provider: "quantum"}); err == nil {
			t.Fatal("want error")
		}
	})

	t.Run("missing local model names the pull command", func(t *testing.T) {
		_, err := NewLocal("does-not-exist-model")
		if err == nil || !strings.Contains(err.Error(), "model pull") {
			t.Fatalf("err = %v, want pull hint", err)
		}
	})
}
