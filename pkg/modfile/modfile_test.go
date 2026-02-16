package modfile_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/organic-programming/rhizome-atlas/pkg/modfile"
)

func TestParseAndWrite(t *testing.T) {
	dir := t.TempDir()
	modPath := filepath.Join(dir, "holon.mod")

	content := `holon github.com/org/myholon

require (
    github.com/org/dep-a v1.2.0
    github.com/org/dep-b v0.5.0
)

replace (
    github.com/org/dep-a => ../local-dep-a
)
`
	if err := os.WriteFile(modPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	mod, err := modfile.Parse(modPath)
	if err != nil {
		t.Fatal(err)
	}

	if mod.HolonPath != "github.com/org/myholon" {
		t.Errorf("HolonPath = %q, want %q", mod.HolonPath, "github.com/org/myholon")
	}
	if len(mod.Require) != 2 {
		t.Fatalf("Require len = %d, want 2", len(mod.Require))
	}
	if mod.Require[0].Path != "github.com/org/dep-a" || mod.Require[0].Version != "v1.2.0" {
		t.Errorf("Require[0] = %+v", mod.Require[0])
	}
	if len(mod.Replace) != 1 {
		t.Fatalf("Replace len = %d, want 1", len(mod.Replace))
	}
	if mod.Replace[0].LocalPath != "../local-dep-a" {
		t.Errorf("Replace[0].LocalPath = %q", mod.Replace[0].LocalPath)
	}

	// Round-trip: write and re-parse
	outPath := filepath.Join(dir, "holon2.mod")
	if err := mod.Write(outPath); err != nil {
		t.Fatal(err)
	}
	mod2, err := modfile.Parse(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if mod2.HolonPath != mod.HolonPath {
		t.Errorf("round-trip HolonPath mismatch")
	}
	if len(mod2.Require) != len(mod.Require) {
		t.Errorf("round-trip Require mismatch")
	}
}

func TestAddRemoveRequire(t *testing.T) {
	mod := &modfile.ModFile{HolonPath: "test/holon"}

	// Add
	if !mod.AddRequire("dep/a", "v1.0.0") {
		t.Error("AddRequire should return true for new dep")
	}
	if len(mod.Require) != 1 {
		t.Fatalf("Require len = %d", len(mod.Require))
	}

	// Update
	if mod.AddRequire("dep/a", "v2.0.0") {
		t.Error("AddRequire should return false for update")
	}
	if mod.Require[0].Version != "v2.0.0" {
		t.Errorf("Version = %q after update", mod.Require[0].Version)
	}

	// Remove
	if !mod.RemoveRequire("dep/a") {
		t.Error("RemoveRequire should return true")
	}
	if len(mod.Require) != 0 {
		t.Error("Require should be empty after remove")
	}
	if mod.RemoveRequire("dep/a") {
		t.Error("RemoveRequire should return false for missing dep")
	}
}

func TestSumRoundTrip(t *testing.T) {
	dir := t.TempDir()
	sumPath := filepath.Join(dir, "holon.sum")

	sum := &modfile.SumFile{}
	sum.Set("dep/a", "v1.0.0", "h1:abc123")
	sum.Set("dep/a", "v1.0.0/HOLON.md", "h1:def456")
	sum.Set("dep/b", "v2.0.0", "h1:ghi789")

	if err := sum.Write(sumPath); err != nil {
		t.Fatal(err)
	}

	sum2, err := modfile.ParseSum(sumPath)
	if err != nil {
		t.Fatal(err)
	}

	if len(sum2.Entries) != 3 {
		t.Fatalf("Entries len = %d, want 3", len(sum2.Entries))
	}

	if h := sum2.Lookup("dep/a", "v1.0.0"); h != "h1:abc123" {
		t.Errorf("Lookup = %q", h)
	}
	if h := sum2.Lookup("dep/a", "v1.0.0/HOLON.md"); h != "h1:def456" {
		t.Errorf("Lookup HOLON.md = %q", h)
	}
}

func TestParseSumMissing(t *testing.T) {
	sum, err := modfile.ParseSum("/nonexistent/holon.sum")
	if err != nil {
		t.Fatal(err)
	}
	if len(sum.Entries) != 0 {
		t.Error("missing file should return empty SumFile")
	}
}
