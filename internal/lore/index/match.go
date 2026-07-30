package index

import (
	"strings"

	"codeindex/internal/lore"
)

// AnchorMatches reports whether record anchor a covers query anchor q:
// symbols match exactly; paths match on either-direction prefix.
func AnchorMatches(a lore.Anchor, q string) bool {
	if a.Symbol != "" {
		return a.Symbol == q
	}
	ap := strings.TrimSuffix(a.Path, "/")
	qp := strings.TrimSuffix(q, "/")
	return ap != "" && (strings.HasPrefix(qp, ap) || strings.HasPrefix(ap, qp))
}

// RecordsForAnchor filters records to those with an anchor covering q.
func RecordsForAnchor(recs []StoredRecord, q string) []StoredRecord {
	var out []StoredRecord
	for _, r := range recs {
		for _, a := range r.Anchors {
			if AnchorMatches(a, q) {
				out = append(out, r)
				break
			}
		}
	}
	return out
}
