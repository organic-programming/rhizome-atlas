// Package server implements the RhizomeAtlasService gRPC server.
package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Organic-Programming/go-holons/pkg/serve"
	"github.com/Organic-Programming/rhizome-atlas/pkg/modfile"
	pb "github.com/Organic-Programming/rhizome-atlas/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// CacheDir returns the global holon cache directory.
func CacheDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".holon", "cache")
}

// Server implements the RhizomeAtlasService.
type Server struct {
	pb.UnimplementedRhizomeAtlasServiceServer
}

// ListenAndServe starts the gRPC server on the given transport URI.
func ListenAndServe(listenURI string, reflection bool) error {
	return serve.RunWithOptions(listenURI, func(s *grpc.Server) {
		pb.RegisterRhizomeAtlasServiceServer(s, &Server{})
	}, reflection)
}

// Init creates a holon.mod file in the given directory.
func (s *Server) Init(_ context.Context, req *pb.InitRequest) (*pb.InitResponse, error) {
	dir := req.Directory
	if dir == "" {
		dir = "."
	}
	holonPath := req.HolonPath
	if holonPath == "" {
		return nil, status.Error(codes.InvalidArgument, "holon_path is required")
	}

	modPath := filepath.Join(dir, "holon.mod")
	if _, err := os.Stat(modPath); err == nil {
		return nil, status.Errorf(codes.AlreadyExists, "holon.mod already exists in %s", dir)
	}

	mod := &modfile.ModFile{HolonPath: holonPath}
	if err := mod.Write(modPath); err != nil {
		return nil, status.Errorf(codes.Internal, "write holon.mod: %v", err)
	}

	return &pb.InitResponse{ModFile: modPath}, nil
}

// Add adds a dependency to holon.mod and fetches it to the cache.
func (s *Server) Add(_ context.Context, req *pb.AddRequest) (*pb.AddResponse, error) {
	dir := req.Directory
	if dir == "" {
		dir = "."
	}

	modPath := filepath.Join(dir, "holon.mod")
	mod, err := modfile.Parse(modPath)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "parse holon.mod: %v", err)
	}

	mod.AddRequire(req.Path, req.Version)

	if err := mod.Write(modPath); err != nil {
		return nil, status.Errorf(codes.Internal, "write holon.mod: %v", err)
	}

	// Fetch immediately
	cachePath, err := fetchToCache(req.Path, req.Version)
	if err != nil {
		log.Printf("atlas: fetch %s@%s: %v (added to holon.mod, fetch deferred)", req.Path, req.Version, err)
		cachePath = "" // not fatal — dependency is recorded
	}

	// Update holon.sum
	if cachePath != "" {
		sumPath := filepath.Join(dir, "holon.sum")
		sum, _ := modfile.ParseSum(sumPath)
		hash, _ := hashDir(cachePath)
		if hash != "" {
			sum.Set(req.Path, req.Version, "h1:"+hash)
		}
		holonMDHash, _ := hashFile(filepath.Join(cachePath, "HOLON.md"))
		if holonMDHash != "" {
			sum.Set(req.Path, req.Version+"/HOLON.md", "h1:"+holonMDHash)
		}
		sum.Write(sumPath) //nolint:errcheck
	}

	return &pb.AddResponse{
		Dependency: &pb.Dependency{
			Path:      req.Path,
			Version:   req.Version,
			CachePath: cachePath,
		},
	}, nil
}

// Remove removes a dependency from holon.mod.
func (s *Server) Remove(_ context.Context, req *pb.RemoveRequest) (*pb.RemoveResponse, error) {
	dir := req.Directory
	if dir == "" {
		dir = "."
	}

	modPath := filepath.Join(dir, "holon.mod")
	mod, err := modfile.Parse(modPath)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "parse holon.mod: %v", err)
	}

	if !mod.RemoveRequire(req.Path) {
		return nil, status.Errorf(codes.NotFound, "dependency %q not found in holon.mod", req.Path)
	}

	if err := mod.Write(modPath); err != nil {
		return nil, status.Errorf(codes.Internal, "write holon.mod: %v", err)
	}

	return &pb.RemoveResponse{}, nil
}

// Pull fetches all dependencies to the cache and updates holon.sum.
func (s *Server) Pull(_ context.Context, req *pb.PullRequest) (*pb.PullResponse, error) {
	dir := req.Directory
	if dir == "" {
		dir = "."
	}

	modPath := filepath.Join(dir, "holon.mod")
	mod, err := modfile.Parse(modPath)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "parse holon.mod: %v", err)
	}

	sumPath := filepath.Join(dir, "holon.sum")
	sum, _ := modfile.ParseSum(sumPath)

	var fetched []*pb.Dependency
	for _, req := range mod.Require {
		// Skip replaced dependencies
		if mod.ResolvedPath(req.Path) != "" {
			continue
		}

		cachePath, err := fetchToCache(req.Path, req.Version)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "fetch %s@%s: %v", req.Path, req.Version, err)
		}

		hash, _ := hashDir(cachePath)
		if hash != "" {
			sum.Set(req.Path, req.Version, "h1:"+hash)
		}
		holonMDHash, _ := hashFile(filepath.Join(cachePath, "HOLON.md"))
		if holonMDHash != "" {
			sum.Set(req.Path, req.Version+"/HOLON.md", "h1:"+holonMDHash)
		}

		fetched = append(fetched, &pb.Dependency{
			Path:      req.Path,
			Version:   req.Version,
			CachePath: cachePath,
		})
	}

	if err := sum.Write(sumPath); err != nil {
		return nil, status.Errorf(codes.Internal, "write holon.sum: %v", err)
	}

	return &pb.PullResponse{Fetched: fetched}, nil
}

// Verify checks holon.sum integrity against cached content.
func (s *Server) Verify(_ context.Context, req *pb.VerifyRequest) (*pb.VerifyResponse, error) {
	dir := req.Directory
	if dir == "" {
		dir = "."
	}

	sumPath := filepath.Join(dir, "holon.sum")
	sum, err := modfile.ParseSum(sumPath)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "parse holon.sum: %v", err)
	}

	// Also check for active replaces
	modPath := filepath.Join(dir, "holon.mod")
	mod, _ := modfile.Parse(modPath)

	var errors []string

	if mod != nil && len(mod.Replace) > 0 {
		for _, r := range mod.Replace {
			errors = append(errors, fmt.Sprintf("WARNING: active replace %s => %s", r.Old, r.LocalPath))
		}
	}

	for _, entry := range sum.Entries {
		// Extract base version (strip /HOLON.md suffix)
		version := entry.Version
		isHolonMD := strings.HasSuffix(version, "/HOLON.md")
		if isHolonMD {
			version = strings.TrimSuffix(version, "/HOLON.md")
		}

		cachePath := cachePathFor(entry.Path, version)

		var currentHash string
		if isHolonMD {
			currentHash, _ = hashFile(filepath.Join(cachePath, "HOLON.md"))
		} else {
			currentHash, _ = hashDir(cachePath)
		}

		if currentHash == "" {
			errors = append(errors, fmt.Sprintf("%s %s: not in cache", entry.Path, entry.Version))
		} else if "h1:"+currentHash != entry.Hash {
			errors = append(errors, fmt.Sprintf("%s %s: hash mismatch (want %s, got h1:%s)",
				entry.Path, entry.Version, entry.Hash, currentHash))
		}
	}

	return &pb.VerifyResponse{
		Ok:     len(errors) == 0,
		Errors: errors,
	}, nil
}

// Graph returns the dependency tree.
func (s *Server) Graph(_ context.Context, req *pb.GraphRequest) (*pb.GraphResponse, error) {
	dir := req.Directory
	if dir == "" {
		dir = "."
	}

	modPath := filepath.Join(dir, "holon.mod")
	mod, err := modfile.Parse(modPath)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "parse holon.mod: %v", err)
	}

	var edges []*pb.Edge
	for _, req := range mod.Require {
		edges = append(edges, &pb.Edge{
			From:    mod.HolonPath,
			To:      req.Path,
			Version: req.Version,
		})

		// Recurse into cached dependencies
		cachePath := cachePathFor(req.Path, req.Version)
		subModPath := filepath.Join(cachePath, "holon.mod")
		if subMod, err := modfile.Parse(subModPath); err == nil {
			for _, sub := range subMod.Require {
				edges = append(edges, &pb.Edge{
					From:    req.Path,
					To:      sub.Path,
					Version: sub.Version,
				})
			}
		}
	}

	return &pb.GraphResponse{
		Root:  mod.HolonPath,
		Edges: edges,
	}, nil
}

// --- helpers ---

// cachePathFor returns the cache directory for a dependency.
func cachePathFor(depPath, version string) string {
	return filepath.Join(CacheDir(), depPath+"@"+version)
}

// fetchToCache clones/fetches a holon to the global cache.
func fetchToCache(depPath, version string) (string, error) {
	cachePath := cachePathFor(depPath, version)

	// Already cached?
	if info, err := os.Stat(cachePath); err == nil && info.IsDir() {
		return cachePath, nil
	}

	// Construct git URL from path
	gitURL := "https://" + depPath + ".git"

	// Clone at the specific tag
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		return "", fmt.Errorf("create cache dir: %w", err)
	}

	cmd := exec.Command("git", "clone", "--depth=1", "--branch", version, gitURL, cachePath)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		// Try without .git suffix
		gitURL = "https://" + depPath
		cmd = exec.Command("git", "clone", "--depth=1", "--branch", version, gitURL, cachePath)
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("git clone %s@%s: %w", depPath, version, err)
		}
	}

	// Remove .git directory — cache is read-only snapshots
	os.RemoveAll(filepath.Join(cachePath, ".git")) //nolint:errcheck

	return cachePath, nil
}

// hashDir computes SHA-256 of all files in a directory.
func hashDir(dir string) (string, error) {
	h := sha256.New()
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		// Write relative path for reproducibility
		rel, _ := filepath.Rel(dir, path)
		h.Write([]byte(rel))

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		h.Write(data)
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// hashFile computes SHA-256 of a single file.
func hashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:]), nil
}
