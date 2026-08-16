// C shim over llama.cpp's embedding API. Kept deliberately tiny: load a GGUF
// embedding model, embed one text (mean-pooled, caller normalizes), free.
// The Go side serializes calls per handle (llama_context is not thread-safe);
// parallelism comes from ggml's internal thread pool.
#ifndef CODEINDEX_EMBED_SHIM_H
#define CODEINDEX_EMBED_SHIM_H

#ifdef __cplusplus
extern "C" {
#endif

// ci_embed_load returns an opaque handle, or NULL on failure.
void *ci_embed_load(const char *model_path, int n_threads);

// ci_embed_clone returns a new handle sharing the loaded model weights but
// with its own llama_context (own thread pool, own state) — the unit of
// embed parallelism. The clone must be freed before (or after) the parent;
// the shared model is freed exactly once, by the loading handle's free.
// Returns NULL on failure.
void *ci_embed_clone(void *handle, int n_threads);

// ci_embed_dims returns the embedding dimensionality of the loaded model.
int ci_embed_dims(void *handle);

// ci_embed_text embeds one UTF-8 text into out (ci_embed_dims floats,
// mean-pooled, unnormalized). Texts longer than the context window are
// truncated. Returns 0 on success, negative on failure.
int ci_embed_text(void *handle, const char *text, float *out);

// ci_embed_batch embeds count texts into out (count * ci_embed_dims floats),
// packing as many sequences per llama_encode call as the batch budget
// allows — the throughput path for index builds. Per-text truncation as
// above. Returns 0 on success, negative on failure.
int ci_embed_batch(void *handle, const char **texts, int count, float *out);

void ci_embed_free(void *handle);

#ifdef __cplusplus
}
#endif

#endif
