//go:build !nollama

// See shim.h. Compiled only in cgo builds that link the vendored llama.cpp
// static libraries (scripts/vendor-llama.sh).
#include "shim.h"

#include <stdlib.h>
#include <string.h>

#include "llama.h"

typedef struct {
	struct llama_model *model;
	struct llama_context *ctx;
	const struct llama_vocab *vocab;
	int n_ctx;
	int dims;
	int owns_model; // clones share the parent's model; only the owner frees it
} ci_handle;

static void ci_log_silent(enum ggml_log_level level, const char *text, void *user) {
	(void)level; (void)text; (void)user;
}

// ci_ctx_new builds an embedding context on an already-loaded model.
static struct llama_context *ci_ctx_new(struct llama_model *model, int n_threads) {
	struct llama_context_params cp = llama_context_default_params();
	cp.embeddings = true;
	// Batch geometry: many short card sequences packed per encode call.
	// Non-causal encoders process the whole batch in one ubatch.
	cp.n_ctx = 2048;
	cp.n_batch = 2048;
	cp.n_ubatch = 2048;
	cp.n_seq_max = 32;
	cp.pooling_type = LLAMA_POOLING_TYPE_MEAN;
	// Small encoders lose to thread-sync overhead past a handful of cores.
	if (n_threads > 8) n_threads = 8;
	cp.n_threads = n_threads;
	cp.n_threads_batch = n_threads;
	return llama_init_from_model(model, cp);
}

static ci_handle *ci_handle_new(struct llama_model *model, struct llama_context *ctx, int owns_model) {
	ci_handle *h = calloc(1, sizeof(ci_handle));
	h->model = model;
	h->ctx = ctx;
	h->vocab = llama_model_get_vocab(model);
	h->n_ctx = (int)llama_n_ctx(ctx);
	h->dims = llama_model_n_embd(model);
	h->owns_model = owns_model;
	return h;
}

void *ci_embed_load(const char *model_path, int n_threads) {
	// Model-load/inference chatter would drown the CLI and MCP stderr.
	llama_log_set(ci_log_silent, NULL);
	llama_backend_init();

	struct llama_model_params mp = llama_model_default_params();
	struct llama_model *model = llama_model_load_from_file(model_path, mp);
	if (model == NULL) {
		return NULL;
	}
	struct llama_context *ctx = ci_ctx_new(model, n_threads);
	if (ctx == NULL) {
		llama_model_free(model);
		return NULL;
	}
	return ci_handle_new(model, ctx, /*owns_model=*/1);
}

void *ci_embed_clone(void *handle, int n_threads) {
	ci_handle *parent = (ci_handle *)handle;
	struct llama_context *ctx = ci_ctx_new(parent->model, n_threads);
	if (ctx == NULL) {
		return NULL;
	}
	return ci_handle_new(parent->model, ctx, /*owns_model=*/0);
}

int ci_embed_dims(void *handle) {
	return ((ci_handle *)handle)->dims;
}

int ci_embed_text(void *handle, const char *text, float *out) {
	ci_handle *h = (ci_handle *)handle;

	int cap = h->n_ctx;
	llama_token *toks = malloc(sizeof(llama_token) * cap);
	int n = llama_tokenize(h->vocab, text, (int32_t)strlen(text), toks, cap,
	                       /*add_special=*/true, /*parse_special=*/true);
	if (n < 0) {
		// Needed more than cap tokens: tokenize again and truncate. Keep the
		// final special token if the tokenizer appends one (BERT [SEP]).
		n = cap;
	}
	if (n == 0) {
		free(toks);
		return -1;
	}

	struct llama_batch batch = llama_batch_init(n, 0, 1);
	for (int i = 0; i < n; i++) {
		batch.token[i] = toks[i];
		batch.pos[i] = i;
		batch.n_seq_id[i] = 1;
		batch.seq_id[i][0] = 0;
		batch.logits[i] = true;
	}
	batch.n_tokens = n;
	free(toks);

	// Decoder models accumulate KV state across calls; clear between texts.
	llama_memory_t mem = llama_get_memory(h->ctx);
	if (mem != NULL) {
		llama_memory_clear(mem, true);
	}

	int rc;
	if (llama_model_has_encoder(h->model) && !llama_model_has_decoder(h->model)) {
		rc = llama_encode(h->ctx, batch);
	} else {
		rc = llama_decode(h->ctx, batch);
	}
	if (rc != 0) {
		llama_batch_free(batch);
		return -2;
	}

	float *emb = llama_get_embeddings_seq(h->ctx, 0);
	if (emb == NULL) {
		llama_batch_free(batch);
		return -3;
	}
	memcpy(out, emb, sizeof(float) * (size_t)h->dims);
	llama_batch_free(batch);
	return 0;
}

// per-sequence token cap inside a packed batch: cards are short; 256 tokens
// covers them and lets ~8 sequences share one 2048-token encode call.
#define CI_SEQ_TOKENS 256
// must match cp.n_seq_max at context init
#define CI_MAX_SEQ 32

int ci_embed_batch(void *handle, const char **texts, int count, float *out) {
	ci_handle *h = (ci_handle *)handle;
	int budget = h->n_ctx; // n_batch == n_ctx (set at load)

	llama_token *toks = malloc(sizeof(llama_token) * CI_SEQ_TOKENS);
	struct llama_batch batch = llama_batch_init(budget, 0, /*seq ids per token=*/1);

	int next = 0; // next text to pack
	while (next < count) {
		batch.n_tokens = 0;
		int first = next, seq = 0;

		while (next < count && seq < CI_MAX_SEQ) {
			int cap = CI_SEQ_TOKENS;
			const char *text = (texts[next] != NULL && texts[next][0] != '\0') ? texts[next] : " ";
			int n = llama_tokenize(h->vocab, text, (int32_t)strlen(text),
			                       toks, cap, true, true);
			if (n < 0) n = cap;
			if (n <= 0) {
				llama_batch_free(batch);
				free(toks);
				return -4;
			}
			if (batch.n_tokens + n > budget && batch.n_tokens > 0) {
				break; // batch full; encode what we have
			}
			for (int i = 0; i < n; i++) {
				int k = batch.n_tokens + i;
				batch.token[k] = toks[i];
				batch.pos[k] = i;
				batch.n_seq_id[k] = 1;
				batch.seq_id[k][0] = seq;
				batch.logits[k] = true;
			}
			batch.n_tokens += n;
			seq++;
			next++;
		}

		llama_memory_t mem = llama_get_memory(h->ctx);
		if (mem != NULL) {
			llama_memory_clear(mem, true);
		}
		int rc;
		if (llama_model_has_encoder(h->model) && !llama_model_has_decoder(h->model)) {
			rc = llama_encode(h->ctx, batch);
		} else {
			rc = llama_decode(h->ctx, batch);
		}
		if (rc != 0) {
			llama_batch_free(batch);
			free(toks);
			return -2;
		}
		for (int s = 0; s < seq; s++) {
			float *emb = llama_get_embeddings_seq(h->ctx, s);
			if (emb == NULL) {
				llama_batch_free(batch);
				free(toks);
				return -3;
			}
			memcpy(out + (size_t)(first + s) * h->dims, emb, sizeof(float) * (size_t)h->dims);
		}
	}
	llama_batch_free(batch);
	free(toks);
	return 0;
}

void ci_embed_free(void *handle) {
	ci_handle *h = (ci_handle *)handle;
	if (h == NULL) {
		return;
	}
	llama_free(h->ctx);
	if (h->owns_model) {
		llama_model_free(h->model);
	}
	free(h);
}
