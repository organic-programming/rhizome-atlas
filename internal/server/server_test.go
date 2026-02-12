package server_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Organic-Programming/rhizome-atlas/internal/server"
	pb "github.com/Organic-Programming/rhizome-atlas/proto"
)

func TestInitAddRemoveGraph(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	srv := &server.Server{}

	// Init
	initResp, err := srv.Init(ctx, &pb.InitRequest{
		Directory: dir,
		HolonPath: "github.com/test/myholon",
	})
	if err != nil {
		t.Fatal(err)
	}
	if initResp.ModFile == "" {
		t.Error("expected mod_file path")
	}
	if _, err := os.Stat(filepath.Join(dir, "holon.mod")); err != nil {
		t.Fatal("holon.mod not created")
	}

	// Init again should fail
	_, err = srv.Init(ctx, &pb.InitRequest{
		Directory: dir,
		HolonPath: "github.com/test/myholon",
	})
	if err == nil {
		t.Error("expected error for duplicate init")
	}

	// Add (local dep — will fail fetch but still record in holon.mod)
	addResp, err := srv.Add(ctx, &pb.AddRequest{
		Directory: dir,
		Path:      "github.com/test/fake-dep",
		Version:   "v0.1.0",
	})
	if err != nil {
		t.Fatal(err)
	}
	if addResp.Dependency.Path != "github.com/test/fake-dep" {
		t.Errorf("dep path = %q", addResp.Dependency.Path)
	}

	// Graph should show the edge
	graphResp, err := srv.Graph(ctx, &pb.GraphRequest{Directory: dir})
	if err != nil {
		t.Fatal(err)
	}
	if graphResp.Root != "github.com/test/myholon" {
		t.Errorf("root = %q", graphResp.Root)
	}
	if len(graphResp.Edges) != 1 {
		t.Fatalf("edges = %d, want 1", len(graphResp.Edges))
	}

	// Remove
	_, err = srv.Remove(ctx, &pb.RemoveRequest{
		Directory: dir,
		Path:      "github.com/test/fake-dep",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Graph should be empty
	graphResp, err = srv.Graph(ctx, &pb.GraphRequest{Directory: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(graphResp.Edges) != 0 {
		t.Errorf("edges after remove = %d", len(graphResp.Edges))
	}
}

func TestVerifyEmpty(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	srv := &server.Server{}

	// Init and verify (no deps = all ok)
	srv.Init(ctx, &pb.InitRequest{Directory: dir, HolonPath: "test/h"}) //nolint:errcheck

	resp, err := srv.Verify(ctx, &pb.VerifyRequest{Directory: dir})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Ok {
		t.Errorf("verify should be ok with no deps, got errors: %v", resp.Errors)
	}
}

func TestVendorAndCleanCache(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	srv := &server.Server{}

	// Setup: init, add a real dep (go-holons has a v0.1.0 tag)
	srv.Init(ctx, &pb.InitRequest{Directory: dir, HolonPath: "test/vendor"}) //nolint:errcheck
	srv.Add(ctx, &pb.AddRequest{
		Directory: dir,
		Path:      "github.com/Organic-Programming/go-holons",
		Version:   "v0.1.0",
	}) //nolint:errcheck

	// Vendor
	vendorResp, err := srv.Vendor(ctx, &pb.VendorRequest{Directory: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(vendorResp.Vendored) != 1 {
		t.Fatalf("vendored = %d, want 1", len(vendorResp.Vendored))
	}

	// Check .holon/go-holons/ exists
	vendored := filepath.Join(dir, ".holon", "go-holons")
	if _, err := os.Stat(vendored); os.IsNotExist(err) {
		t.Error(".holon/go-holons/ not created")
	}

	// Clean cache
	cacheResp, err := srv.CleanCache(ctx, &pb.CleanCacheRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if cacheResp.CachePath == "" {
		t.Error("expected cache_path in response")
	}

	// Verify cache is gone
	if _, err := os.Stat(server.CacheDir()); !os.IsNotExist(err) {
		t.Error("cache dir should not exist after clean")
	}
}

func TestUpdateNoRemote(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	srv := &server.Server{}

	// Setup with a fake dep (no remote to query)
	srv.Init(ctx, &pb.InitRequest{Directory: dir, HolonPath: "test/up"}) //nolint:errcheck
	srv.Add(ctx, &pb.AddRequest{
		Directory: dir,
		Path:      "github.com/test/nonexistent",
		Version:   "v0.1.0",
	}) //nolint:errcheck

	// Update should not fail — just log and skip
	resp, err := srv.Update(ctx, &pb.UpdateRequest{Directory: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Updated) != 0 {
		t.Errorf("expected 0 updates for unreachable dep, got %d", len(resp.Updated))
	}
}
