// Package search implements ranked symbol search under partial knowledge:
// convention-aware tokenization, deterministic synonym/stem expansion, and a
// match ladder combined with graph signals (caller counts, tier, kind). No
// vector store by design — every ranking is deterministic and explainable.
package search

import "strings"

// Tokenize splits an identifier by naming convention: case humps, digits,
// underscore/dash/dot separators, with acronym-run handling. One splitter
// covers Go/TS/Python/PHP conventions:
//
//	parseConfig      -> [parse config]
//	load_config      -> [load config]
//	HTTPServerConfig -> [http server config]
//	firstOrCreate    -> [first or create]
func Tokenize(name string) []string {
	var tokens []string
	var cur []rune
	runes := []rune(name)
	flush := func() {
		if len(cur) > 0 {
			tokens = append(tokens, strings.ToLower(string(cur)))
			cur = nil
		}
	}
	for i, r := range runes {
		switch {
		case r == '_' || r == '-' || r == '.' || r == '$' || r == ' ':
			flush()
		case isUpper(r):
			prevLower := i > 0 && isLower(runes[i-1])
			prevDigit := i > 0 && isDigit(runes[i-1])
			nextLower := i+1 < len(runes) && isLower(runes[i+1])
			// New token at lower->Upper or digit->Upper boundaries, or at the
			// last capital of an acronym run followed by a lowercase
			// (HTTPServer -> HTTP|Server).
			if prevLower || prevDigit || (nextLower && i > 0 && isUpper(runes[i-1])) {
				flush()
			}
			cur = append(cur, r)
		case isDigit(r):
			if len(cur) > 0 && !isDigit(runes[i-1]) {
				flush()
			}
			cur = append(cur, r)
		default:
			if len(cur) > 0 && isDigit(runes[i-1]) {
				flush() // digit->letter boundary (V4Signer -> 4|signer... -> signer)
			}
			cur = append(cur, r)
		}
	}
	flush()
	return tokens
}

func isUpper(r rune) bool { return r >= 'A' && r <= 'Z' }
func isLower(r rune) bool { return r >= 'a' && r <= 'z' }
func isDigit(r rune) bool { return r >= '0' && r <= '9' }

// synonymGroups is the static, inspectable expansion table (D4). Membership is
// symmetric within a group. Deliberately small and versioned with the code —
// misses show up in the recall benchmark, not inside a model.
var synonymGroups = [][]string{
	{"get", "fetch", "load", "retrieve", "read", "lookup"},
	{"set", "put", "write", "store", "save", "persist"},
	{"init", "setup", "create", "new", "make", "build", "construct"},
	{"delete", "remove", "destroy", "drop", "clear", "purge"},
	{"config", "configuration", "settings", "options", "conf", "opts"},
	{"check", "validate", "verify", "assert", "ensure", "test"},
	{"send", "emit", "publish", "dispatch", "post", "broadcast"},
	{"receive", "consume", "subscribe", "listen", "poll"},
	{"find", "search", "locate", "query", "match"},
	{"handle", "process", "on", "callback", "hook"},
	{"start", "run", "launch", "exec", "execute", "invoke", "begin"},
	{"stop", "halt", "shutdown", "close", "end", "terminate", "kill"},
	{"update", "modify", "change", "edit", "patch", "mutate"},
	{"parse", "decode", "unmarshal", "deserialize", "scan"},
	{"format", "encode", "marshal", "serialize", "render", "print"},
	{"add", "append", "insert", "push", "register", "attach"},
	{"list", "all", "each", "iterate", "enumerate", "collect"},
	{"count", "len", "length", "size", "num", "total"},
	{"copy", "clone", "dup", "duplicate"},
	{"merge", "join", "combine", "concat", "union"},
	{"split", "divide", "partition", "chunk"},
	{"error", "err", "fail", "failure", "fault"},
	{"log", "debug", "trace", "print"},
	{"auth", "authenticate", "login", "signin", "authorize"},
	{"user", "account", "member", "principal"},
	{"request", "req", "call"},
	{"response", "resp", "reply", "result"},
	{"connect", "dial", "open", "attach"},
	{"cache", "memo", "memoize", "buffer"},
	{"wait", "block", "sleep", "pause", "delay"},
	{"retry", "backoff", "reattempt"},
	{"encrypt", "cipher", "seal"},
	{"decrypt", "unseal", "decode"},
	{"compress", "zip", "pack", "deflate"},
	{"path", "file", "filename", "filepath", "dir", "directory", "folder"},
	{"convert", "transform", "translate", "map", "cast", "to"},
	{"filter", "select", "where", "exclude"},
	{"sort", "order", "rank", "arrange"},
	{"compare", "diff", "equal", "cmp", "match"},
	{"watch", "observe", "monitor", "track"},
	{"apply", "commit", "flush", "sync"},
	{"clone", "fork", "spawn", "copy"},
	{"render", "draw", "paint", "display", "show"},
	{"hide", "conceal", "mask", "redact"},
	{"resolve", "lookup", "bind"},
	{"schedule", "queue", "enqueue", "defer"},
	{"lock", "mutex", "acquire", "hold"},
	{"unlock", "release", "free"},
	{"env", "environment", "context", "ctx"},
	{"max", "maximum", "limit", "cap", "bound"},
	{"min", "minimum", "floor"},
}

var synonyms = func() map[string][]string {
	m := map[string][]string{}
	for _, group := range synonymGroups {
		for _, w := range group {
			for _, other := range group {
				if other != w {
					m[w] = append(m[w], other)
				}
			}
		}
	}
	return m
}()

// Stem folds common suffixes so parse/parser/parsing meet in the middle.
func Stem(tok string) string {
	for _, suf := range []string{"ing", "ers", "er", "ed", "es", "s"} {
		if strings.HasSuffix(tok, suf) && len(tok)-len(suf) >= 3 {
			return tok[:len(tok)-len(suf)]
		}
	}
	return tok
}

// Expand returns the query token plus its synonyms (weighted separately by
// the scorer) — stems applied to both sides at comparison time.
func Expand(tok string) []string {
	return synonyms[tok]
}
