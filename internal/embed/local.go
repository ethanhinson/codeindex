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
// concurrent use; calls are serialized per handle and parallelism comes from
// ggml's thread pool.
type local struct {
	mu     sync.Mutex
	handle unsafe.Pointer
	id     string
	dims   int
}

// NewLocal opens the local provider. model "" loads the bundled weights
// (extracted once to the user cache — llama.cpp reads a file path); otherwise
// model is a name in the model cache (see Pull) or a path to a GGUF file.
func NewLocal(model string) (Provider, error) {
	path, id, err := resolveModel(model)
	if err != nil {
		return nil, err
	}
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	h := C.ci_embed_load(cpath, C.int(runtime.NumCPU()))
	if h == nil {
		return nil, fmt.Errorf("%w: failed to load model %s", ErrUnavailable, path)
	}
	return &local{handle: h, id: id, dims: int(C.ci_embed_dims(h))}, nil
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

func (l *local) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.handle == nil {
		return nil, ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

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
	rc := C.ci_embed_batch(l.handle, (**C.char)(unsafe.Pointer(&ctexts[0])),
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
	if l.handle != nil {
		C.ci_embed_free(l.handle)
		l.handle = nil
	}
	return nil
}
