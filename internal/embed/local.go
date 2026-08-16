//go:build !nollama

package embed

/*
#cgo CFLAGS: -I${SRCDIR}/../../third_party/llama.cpp/include -I${SRCDIR}/../../third_party/llama.cpp/ggml/include
#cgo darwin LDFLAGS: ${SRCDIR}/../../third_party/llama.cpp/build/src/libllama.a ${SRCDIR}/../../third_party/llama.cpp/build/ggml/src/libggml.a ${SRCDIR}/../../third_party/llama.cpp/build/ggml/src/libggml-cpu.a ${SRCDIR}/../../third_party/llama.cpp/build/ggml/src/ggml-blas/libggml-blas.a ${SRCDIR}/../../third_party/llama.cpp/build/ggml/src/libggml-base.a -lc++ -framework Accelerate -framework Foundation
#cgo linux LDFLAGS: ${SRCDIR}/../../third_party/llama.cpp/build/src/libllama.a ${SRCDIR}/../../third_party/llama.cpp/build/ggml/src/libggml.a ${SRCDIR}/../../third_party/llama.cpp/build/ggml/src/libggml-cpu.a ${SRCDIR}/../../third_party/llama.cpp/build/ggml/src/libggml-base.a -lstdc++ -lm -lpthread
#include <stdlib.h>
#include "shim.h"
*/
import "C"

import (
	"context"
	"crypto/sha256"
	stdembed "embed"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"unsafe"
)

// BundledModelName identifies the model compiled into the binary.
const BundledModelName = "all-minilm-l6-v2.q8_0"

//go:embed models/all-minilm-l6-v2.q8_0.gguf
var bundledFS stdembed.FS

const bundledPath = "models/all-minilm-l6-v2.q8_0.gguf"

var bundledID = sync.OnceValue(func() string {
	b, _ := bundledFS.ReadFile(bundledPath)
	return fmt.Sprintf("%s@%x", BundledModelName, sha256.Sum256(b))[:len(BundledModelName)+13]
})

// Local runs a GGUF embedding model in-process (llama.cpp, CPU). Safe for
// concurrent use: a pool of llama contexts shares one loaded model, and
// each Embed call checks a context out for its duration — within a context
// parallelism comes from ggml's thread pool, across contexts from the pool.
type local struct {
	mu   sync.Mutex // guards close
	free chan unsafe.Pointer
	all  []unsafe.Pointer // all[0] owns the model; freed last
	id   string
	dims int
}

// poolConfig reads the embed-parallelism knobs. CODEINDEX_EMBED_CTX is the
// number of llama contexts (default 1 — the historical configuration);
// CODEINDEX_EMBED_THREADS is the ggml thread count per context (default
// NumCPU, capped at 8 in the shim — small encoders lose to sync overhead
// past that).
func poolConfig() (ctxs, threads int) {
	ctxs, threads = 1, runtime.NumCPU()
	if v, err := strconv.Atoi(os.Getenv("CODEINDEX_EMBED_CTX")); err == nil && v > 0 && v <= 32 {
		ctxs = v
	}
	if v, err := strconv.Atoi(os.Getenv("CODEINDEX_EMBED_THREADS")); err == nil && v > 0 {
		threads = v
	}
	return ctxs, threads
}

// NewLocal opens the local provider. model "" loads the bundled weights
// (extracted once to the user cache — llama.cpp reads a file path); otherwise
// model is a name in the model cache (see Pull) or a path to a GGUF file.
func NewLocal(model string) (Provider, error) {
	path, id, err := resolveModel(model)
	if err != nil {
		return nil, err
	}
	ctxs, threads := poolConfig()
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	h := C.ci_embed_load(cpath, C.int(threads))
	if h == nil {
		return nil, fmt.Errorf("%w: failed to load model %s", ErrUnavailable, path)
	}
	l := &local{
		free: make(chan unsafe.Pointer, ctxs),
		all:  []unsafe.Pointer{h},
		id:   id,
		dims: int(C.ci_embed_dims(h)),
	}
	l.free <- h
	for i := 1; i < ctxs; i++ {
		c := C.ci_embed_clone(h, C.int(threads))
		if c == nil {
			break // partial pool still works; parallelism just degrades
		}
		l.all = append(l.all, c)
		l.free <- c
	}
	return l, nil
}

// resolveModel maps a model spec to (gguf path, model ID), extracting the
// bundled weights on first use.
func resolveModel(model string) (string, string, error) {
	if model == "" || model == BundledModelName {
		path, err := extractBundled()
		return path, bundledID(), err
	}
	path := model
	if _, err := os.Stat(path); err != nil {
		dir, derr := ModelCacheDir()
		if derr != nil {
			return "", "", derr
		}
		path = filepath.Join(dir, model+".gguf")
		if _, err := os.Stat(path); err != nil {
			return "", "", fmt.Errorf("%w: model %q not found (codeindex model pull %[2]s)", ErrUnavailable, model)
		}
	}
	id, err := fileModelID(path)
	return path, id, err
}

// extractBundled writes the embedded weights to the model cache once and
// returns the path (size mismatch or absence triggers a rewrite).
func extractBundled() (string, error) {
	dir, err := ModelCacheDir()
	if err != nil {
		return "", err
	}
	b, err := bundledFS.ReadFile(bundledPath)
	if err != nil {
		return "", err
	}
	dst := filepath.Join(dir, "bundled-"+BundledModelName+".gguf")
	if st, err := os.Stat(dst); err == nil && st.Size() == int64(len(b)) {
		return dst, nil
	}
	tmp := dst + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return "", err
	}
	return dst, os.Rename(tmp, dst)
}

func (l *local) ID() string { return l.id }
func (l *local) Dims() int  { return l.dims }

// Concurrency reports how many Embed calls can make progress in parallel —
// the pool size. Callers (EmbedPass) use it to size their worker fan-out.
func (l *local) Concurrency() int { return cap(l.free) }

func (l *local) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	var h unsafe.Pointer
	select {
	case h = <-l.free:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if h == nil { // closed
		return nil, ErrUnavailable
	}
	defer func() { l.free <- h }()

	ctexts := make([]*C.char, len(texts))
	for i, t := range texts {
		ctexts[i] = C.CString(t)
	}
	defer func() {
		for _, ct := range ctexts {
			C.free(unsafe.Pointer(ct))
		}
	}()

	flat := make([]float32, len(texts)*l.dims)
	rc := C.ci_embed_batch(h, (**C.char)(unsafe.Pointer(&ctexts[0])),
		C.int(len(texts)), (*C.float)(unsafe.Pointer(&flat[0])))
	if rc != 0 {
		return nil, fmt.Errorf("embed batch of %d: llama rc=%d", len(texts), int(rc))
	}

	out := make([][]float32, len(texts))
	for i := range texts {
		vec := flat[i*l.dims : (i+1)*l.dims : (i+1)*l.dims]
		normalize(vec)
		out[i] = vec
	}
	return out, nil
}

func (l *local) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.all == nil {
		return nil
	}
	// Reclaim every context (waits for in-flight Embeds), then free clones
	// before the owner — the owner's free releases the shared model.
	for range l.all {
		<-l.free
	}
	close(l.free)
	for i := len(l.all) - 1; i >= 0; i-- {
		C.ci_embed_free(l.all[i])
	}
	l.all = nil
	return nil
}
