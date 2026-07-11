package adapter

import "testing"

func TestSniffLang(t *testing.T) {
	cases := []struct {
		name string
		head string
		want string
	}{
		{"php tag", "<?php\nfunction f() {}", "php"},
		{"php short echo", "<?= $x ?>", "php"},
		{"php after BOM+whitespace", "\xEF\xBB\xBF\n  <?php echo 1;", "php"},
		{"php shebang", "#!/usr/bin/env php\n<?php", "php"},
		{"python shebang", "#!/usr/bin/env python3\nimport os", "python"},
		{"node shebang", "#!/usr/bin/env node\nconsole.log(1)", "tsjs"},
		{"bun shebang", "#!/usr/bin/env bun\nexport {}", "tsjs"},
		{"plain text", "hello world", ""},
		{"html", "<!DOCTYPE html><html>", ""},
		{"binary", "ELF\x00\x01\x02", ""},
		{"empty", "", ""},
		{"sh shebang", "#!/bin/sh\necho hi", ""},
	}
	for _, c := range cases {
		if got := SniffLang([]byte(c.head)); got != c.want {
			t.Errorf("%s: SniffLang = %q, want %q", c.name, got, c.want)
		}
	}
}
