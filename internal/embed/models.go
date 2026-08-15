package embed

import (
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Registry maps pullable model names to their GGUF URLs. Larger than the
// bundled default; quality/size trade-offs live in bench findings, not here.
var Registry = map[string]string{
	"bge-small-en-v1.5-q8_0":       "https://huggingface.co/CompendiumLabs/bge-small-en-v1.5-gguf/resolve/main/bge-small-en-v1.5-q8_0.gguf",
	"nomic-embed-text-v1.5-q5_k_m": "https://huggingface.co/nomic-ai/nomic-embed-text-v1.5-GGUF/resolve/main/nomic-embed-text-v1.5.Q5_K_M.gguf",
}

// ModelCacheDir is where pulled and extracted models live.
func ModelCacheDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "codeindex", "models")
	return dir, os.MkdirAll(dir, 0o755)
}

// fileModelID is "<base>@<sha256[:12]>", memoized in a sidecar so repeated
// opens don't rehash large weights.
func fileModelID(path string) (string, error) {
	base := strings.TrimSuffix(filepath.Base(path), ".gguf")
	if sc, err := os.ReadFile(path + ".sha256"); err == nil && len(sc) >= 12 {
		return base + "@" + string(sc[:12]), nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := fmt.Sprintf("%x", sha256.Sum256(b))
	_ = os.WriteFile(path+".sha256", []byte(sum), 0o644)
	return base + "@" + sum[:12], nil
}

// Pull downloads a registry model (or a direct URL, keyed by its final path
// segment) into the model cache. Atomic: tmp + rename, sidecar hash written.
func Pull(name string, progress io.Writer) (string, error) {
	url, ok := Registry[name]
	if !ok {
		if strings.HasPrefix(name, "https://") || strings.HasPrefix(name, "http://") {
			url = name
			name = strings.TrimSuffix(filepath.Base(strings.SplitN(url, "?", 2)[0]), ".gguf")
		} else {
			return "", fmt.Errorf("unknown model %q (known: %s, or pass a URL)", name, strings.Join(RegistryNames(), ", "))
		}
	}
	dir, err := ModelCacheDir()
	if err != nil {
		return "", err
	}
	dst := filepath.Join(dir, name+".gguf")

	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("pull %s: HTTP %s", name, resp.Status)
	}
	tmp, err := os.CreateTemp(dir, name+".*.tmp")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), resp.Body)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return "", err
	}
	if err := os.Rename(tmp.Name(), dst); err != nil {
		return "", err
	}
	_ = os.WriteFile(dst+".sha256", []byte(fmt.Sprintf("%x", h.Sum(nil))), 0o644)
	if progress != nil {
		fmt.Fprintf(progress, "pulled %s (%.1f MB) -> %s\n", name, float64(n)/(1<<20), dst)
	}
	return name, nil
}

// RegistryNames lists pullable model names, sorted.
func RegistryNames() []string {
	names := make([]string, 0, len(Registry))
	for n := range Registry {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// CachedModels lists models present in the cache (pulled or extracted).
func CachedModels() ([]string, error) {
	dir, err := ModelCacheDir()
	if err != nil {
		return nil, err
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range ents {
		if strings.HasSuffix(e.Name(), ".gguf") {
			out = append(out, strings.TrimSuffix(e.Name(), ".gguf"))
		}
	}
	sort.Strings(out)
	return out, nil
}
