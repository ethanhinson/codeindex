package adapter

import "bytes"

// SniffLang inspects a file head (first ~1KB) for language evidence when the
// extension says nothing: PHP open tags and interpreter shebangs. This is
// what makes "PHP can live in .inc, .module, anything" true with zero
// configuration — a Drupal clone indexes correctly out of the box. Returns
// "" when there is no evidence (or the file looks binary).
func SniffLang(head []byte) string {
	if bytes.IndexByte(head, 0) >= 0 {
		return "" // binary
	}
	s := bytes.TrimPrefix(head, []byte{0xEF, 0xBB, 0xBF}) // UTF-8 BOM
	s = bytes.TrimLeft(s, " \t\r\n")
	if bytes.HasPrefix(s, []byte("<?php")) || bytes.HasPrefix(s, []byte("<?=")) {
		return "php"
	}
	if bytes.HasPrefix(s, []byte("#!")) {
		line := s
		if i := bytes.IndexByte(s, '\n'); i >= 0 {
			line = s[:i]
		}
		switch {
		case bytes.Contains(line, []byte("php")):
			return "php"
		case bytes.Contains(line, []byte("python")):
			return "python"
		case bytes.Contains(line, []byte("node")),
			bytes.Contains(line, []byte("deno")),
			bytes.Contains(line, []byte("bun")):
			return "tsjs"
		}
	}
	return ""
}
