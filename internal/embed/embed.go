// Package embed provides text-embedding providers for semantic search.
//
// The default provider runs a small GGUF model bundled inside the binary
// (statically linked llama.cpp, CPU-only) so embedding works air-gapped with
// no runtime installs. Larger local models and hosted APIs plug in behind the
// same interface; vectors are stamped with the provider's model ID so a model
// change invalidates them.
package embed

import (
	"context"
	"errors"
	"fmt"
	"math"
)

// Provider embeds texts into fixed-dimension vectors. Implementations must be
// safe for concurrent use.
type Provider interface {
	// Embed returns one L2-normalized vector per input text.
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	// ID uniquely identifies the model (name + weights hash for local models;
	// provider/model for APIs). Stored with vectors; mismatch = re-embed.
	ID() string
	// Dims is the embedding dimensionality.
	Dims() int
	Close() error
}

// ErrUnavailable marks embedding as unavailable in this build/configuration;
// callers degrade to lexical-only search and disclose it.
var ErrUnavailable = errors.New("embedding unavailable")

// Config selects and parameterizes a provider (loaded from the repo's
// .codeindex/config.json by internal/config).
type Config struct {
	Provider  string // "local" (default) | "api"
	Model     string // local: model name in the cache ("" = bundled); api: model name
	APIBase   string // api: endpoint base URL ("" = provider default)
	APIKeyEnv string // api: env var holding the credential (default VOYAGE_API_KEY/OPENAI_API_KEY by base)
}

// New opens the configured provider. A config selecting "api" without a
// usable credential falls back to local and reports it via fallbackNote.
func New(cfg Config) (p Provider, fallbackNote string, err error) {
	switch cfg.Provider {
	case "", "local":
		p, err = NewLocal(cfg.Model)
		return p, "", err
	case "api":
		p, err = NewAPI(cfg)
		if err == nil {
			return p, "", nil
		}
		if !errors.Is(err, errNoCredential) {
			return nil, "", err
		}
		p, lerr := NewLocal("")
		if lerr != nil {
			return nil, "", errors.Join(err, lerr)
		}
		return p, "api provider selected but no credential found; using local model", nil
	default:
		return nil, "", fmt.Errorf("unknown embed provider %q", cfg.Provider)
	}
}

// normalize scales v to unit length in place (cosine == dot for stored vectors).
func normalize(v []float32) {
	var s float64
	for _, x := range v {
		s += float64(x) * float64(x)
	}
	n := math.Sqrt(s)
	if n == 0 {
		return
	}
	for i := range v {
		v[i] = float32(float64(v[i]) / n)
	}
}
