package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
)

var errNoCredential = errors.New("no API credential")

// api is an OpenAI-compatible /embeddings client (Voyage, OpenAI, and
// compatible gateways share the request/response shape).
type api struct {
	base  string
	key   string
	model string
	dims  int
	http  *http.Client
}

const (
	defaultAPIBase  = "https://api.voyageai.com/v1"
	defaultAPIModel = "voyage-code-3"
)

// NewAPI opens the hosted provider from config. Returns errNoCredential when
// the key env var is empty (caller falls back to local and disclose).
func NewAPI(cfg Config) (Provider, error) {
	base := cfg.APIBase
	if base == "" {
		base = defaultAPIBase
	}
	keyEnv := cfg.APIKeyEnv
	if keyEnv == "" {
		if strings.Contains(base, "openai.com") {
			keyEnv = "OPENAI_API_KEY"
		} else {
			keyEnv = "VOYAGE_API_KEY"
		}
	}
	key := os.Getenv(keyEnv)
	if key == "" {
		return nil, fmt.Errorf("%w: $%s is empty", errNoCredential, keyEnv)
	}
	model := cfg.Model
	if model == "" {
		model = defaultAPIModel
	}
	return &api{base: strings.TrimRight(base, "/"), key: key, model: model, http: http.DefaultClient}, nil
}

func (a *api) ID() string { return "api/" + a.model }
func (a *api) Dims() int  { return a.dims } // 0 until the first call; storage reads the vector length

func (a *api) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	body, err := json.Marshal(map[string]any{"model": a.model, "input": texts})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.base+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.key)
	resp, err := a.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error struct{ Message string } `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&e)
		return nil, fmt.Errorf("embeddings API: HTTP %s %s", resp.Status, e.Error.Message)
	}
	var out struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if len(out.Data) != len(texts) {
		return nil, fmt.Errorf("embeddings API: %d vectors for %d texts", len(out.Data), len(texts))
	}
	vecs := make([][]float32, len(texts))
	for _, d := range out.Data {
		if d.Index < 0 || d.Index >= len(vecs) {
			return nil, fmt.Errorf("embeddings API: bad index %d", d.Index)
		}
		normalize(d.Embedding)
		vecs[d.Index] = d.Embedding
	}
	if len(vecs) > 0 && vecs[0] != nil {
		a.dims = len(vecs[0])
	}
	return vecs, nil
}

func (a *api) Close() error { return nil }
