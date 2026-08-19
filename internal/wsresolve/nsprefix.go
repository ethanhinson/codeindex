// Package wsresolve implements the workspace resolution ladder: resolving
// cross-member import hints against declared member namespaces.
package wsresolve

import "strings"

// nsPrefix reports whether the import hint h falls inside the declared member namespace
// ns, matching only on a namespace boundary: exact equality, or h continuing after ns
// with one of the separators "/", "\", ".". Trailing separators on ns are trimmed first,
// because PSR-4 autoload keys are declared with one ("Symfony\Component\").
func nsPrefix(ns, h string) bool {
	ns = strings.TrimRight(ns, `/\.`)
	if ns == "" || h == "" {
		return false
	}
	if ns == h {
		return true
	}
	rest, ok := strings.CutPrefix(h, ns)
	if !ok {
		return false
	}
	switch rest[0] {
	case '/', '\\', '.':
		return true
	default:
		return false
	}
}
