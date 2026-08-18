package workspace

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestNamespaces(t *testing.T) {
	cases := []struct {
		name string
		dir  string
		want []string
	}{
		{
			name: "go module path via modfile",
			dir:  "gomod",
			want: []string{"example.com/acme/widget"},
		},
		{
			name: "package.json name",
			dir:  "tsjs",
			want: []string{"@nestjs/core"},
		},
		{
			// Guards Assumption 11: the psr-4 value is string-or-array.
			// A map[string]string decode fails outright on the array value.
			name: "composer name plus psr-4 keys including multi-path array",
			dir:  "php",
			want: []string{"Illuminate\\", "Illuminate\\Support\\", "laravel/framework"},
		},
		{
			// Guards Assumption 12: src/ wins; the root-level flaskroot
			// package must NOT appear.
			name: "python src layout probes src first",
			dir:  "pysrc",
			want: []string{"single", "werkzeug"},
		},
		{
			name: "python root fallback when no src dir",
			dir:  "pyroot",
			want: []string{"mypkg", "setup"},
		},
		{
			name: "no marker files is not an error",
			dir:  "empty",
			want: nil,
		},
		{
			name: "duplicates across probes are deduplicated",
			dir:  "combo",
			want: []string{"example.com/combo"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Namespaces(filepath.Join("testdata", tc.dir))
			if err != nil {
				t.Fatalf("Namespaces: unexpected error: %v", err)
			}
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Namespaces = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNamespacesMissingRootIsNotAnError(t *testing.T) {
	got, err := Namespaces(filepath.Join("testdata", "does-not-exist"))
	if err != nil {
		t.Fatalf("Namespaces: unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("Namespaces = %q, want empty", got)
	}
}

func TestNamespacesMalformedComposerIsAnError(t *testing.T) {
	if _, err := Namespaces(filepath.Join("testdata", "badphp")); err == nil {
		t.Fatal("Namespaces: want error for malformed composer.json, got nil")
	}
}
