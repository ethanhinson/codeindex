package wsresolve

import "testing"

type nsPrefixCase struct {
	name string
	ns   string
	h    string
	want bool
}

// nsPrefixCases is the shared table of nsPrefix boundary-matching cases.
// Later tasks (rung 1 of the ladder, suppression detection) reuse this table
// to assert their own call sites against the same boundary rule.
func nsPrefixCases() []nsPrefixCase {
	return []nsPrefixCase{
		{"go subpackage", "github.com/acme/auth", "github.com/acme/auth/token", true},
		{"go exact", "github.com/acme/auth", "github.com/acme/auth", true},
		{"go lexical false-friend", "github.com/acme/auth", "github.com/acme/authz", false},
		{"psr4 trailing backslash", `Symfony\Component\`, `Symfony\Component\Console`, true},
		{"php namespace subpath", `Acme\Auth`, `Acme\Auth\Token`, true},
		{"python dotted", "flask", "flask.helpers", true},
		{"python lexical false-friend", "flask", "flasky", false},
		{"npm scoped package subpath", "@nestjs/common", "@nestjs/common/decorators", true},

		{"empty ns", "", "flask.helpers", false},
		{"empty hint", "flask", "", false},
		{"both empty", "", "", false},
		{"ns longer than hint", "github.com/acme/auth/token", "github.com/acme/auth", false},
		{"separator-only difference", "flask", "flask/", true},
		{"ns trailing slash trimmed then matches", "github.com/acme/auth/", "github.com/acme/auth/token", true},
		{"ns trailing slash exact after trim", "github.com/acme/auth/", "github.com/acme/auth", true},
	}
}

func TestNsPrefix(t *testing.T) {
	for _, c := range nsPrefixCases() {
		t.Run(c.name, func(t *testing.T) {
			if got := nsPrefix(c.ns, c.h); got != c.want {
				t.Errorf("nsPrefix(%q, %q) = %v, want %v", c.ns, c.h, got, c.want)
			}
		})
	}
}
