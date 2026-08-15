//go:build nollama

package embed

// BundledModelName identifies the model compiled into full builds. The
// nollama build tag strips llama.cpp and the bundled weights (used for
// lightweight CI); local embedding reports ErrUnavailable and search
// degrades to lexical-only.
const BundledModelName = "all-minilm-l6-v2.q8_0"

// NewLocal is unavailable in nollama builds.
func NewLocal(model string) (Provider, error) {
	return nil, ErrUnavailable
}
