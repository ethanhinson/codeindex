package engine

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"codeindex/internal/graph"
)

// Export freshens the index for root (full build if absent, incremental
// patch otherwise) and writes a compact, consistent snapshot to out — the
// shareable artifact a CI job uploads. The artifact is the database itself:
// self-describing via PRAGMA user_version, portable via repo-relative paths.
func Export(root, out string) (Stats, error) {
	db := indexPath(root)
	if err := os.MkdirAll(filepath.Dir(db), 0o755); err != nil {
		return Stats{}, err
	}
	var st Stats
	var err error
	if _, statErr := os.Stat(db); os.IsNotExist(statErr) {
		st, err = Build(root, db)
	} else {
		st, err = Patch(root, db)
	}
	if err != nil {
		return Stats{}, err
	}
	if err := os.Remove(out); err != nil && !os.IsNotExist(err) {
		return Stats{}, err
	}
	if err := graph.VacuumInto(db, out); err != nil {
		return Stats{}, err
	}
	return st, nil
}

// Import installs an exported artifact as root's index and patches it to the
// current tree. A schema-version mismatch is rejected loudly — a stale CI
// artifact should be re-exported, not silently rebuilt over. The returned
// Stats are the drift: how much of the tree the artifact did NOT cover.
func Import(root, artifact string) (Stats, error) {
	artVer, err := graph.FileSchemaVersion(artifact)
	if err != nil {
		return Stats{}, fmt.Errorf("reading artifact %s: %w", artifact, err)
	}
	if artVer != graph.SchemaVersion() {
		return Stats{}, fmt.Errorf(
			"artifact %s has schema v%d but this binary uses v%d — re-export with a matching codeindex (or run codeindex build)",
			artifact, artVer, graph.SchemaVersion())
	}
	db := indexPath(root)
	if err := os.MkdirAll(filepath.Dir(db), 0o755); err != nil {
		return Stats{}, err
	}
	if err := copyFile(artifact, db); err != nil {
		return Stats{}, err
	}
	return Patch(root, db)
}

func indexPath(root string) string {
	return filepath.Join(root, ".codeindex", "graph.db")
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
