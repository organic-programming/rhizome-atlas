// Package server implements the RhizomeAtlasService gRPC server.
package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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

// Update checks remote git tags for each dependency and updates to the
// latest compatible semver version. Follows Minimum Version Selection:
// the latest tag that shares the same major version.
func (s *Server) Update(_ context.Context, req *pb.UpdateRequest) (*pb.UpdateResponse, error) {
	dir := req.Directory
	if dir == "" {
		dir = "."
	}

	modPath := filepath.Join(dir, "holon.mod")
	mod, err := modfile.Parse(modPath)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "parse holon.mod: %v", err)
	}

	var updated []*pb.UpdatedDependency
	for i, dep := range mod.Require {
		// Skip replaced dependencies
		if mod.ResolvedPath(dep.Path) != "" {
			continue
		}

		latest, err := latestCompatibleTag(dep.Path, dep.Version)
		if err != nil {
			log.Printf("atlas update: %s: %v (skipped)", dep.Path, err)
			continue
		}
		if latest == dep.Version {
			continue
		}

		// Remove old cache entry, fetch new
		oldCache := cachePathFor(dep.Path, dep.Version)
		os.RemoveAll(oldCache) //nolint:errcheck

		mod.Require[i].Version = latest
		updated = append(updated, &pb.UpdatedDependency{
			Path:       dep.Path,
			OldVersion: dep.Version,
			NewVersion: latest,
		})
	}

	if len(updated) > 0 {
		if err := mod.Write(modPath); err != nil {
			return nil, status.Errorf(codes.Internal, "write holon.mod: %v", err)
		}
	}

	return &pb.UpdateResponse{Updated: updated}, nil
}

// Vendor copies all cached dependencies to a local .holon/ directory
// next to holon.mod. If .holon/ exists, it is recreated.
func (s *Server) Vendor(_ context.Context, req *pb.VendorRequest) (*pb.VendorResponse, error) {
	dir := req.Directory
	if dir == "" {
		dir = "."
	}

	modPath := filepath.Join(dir, "holon.mod")
	mod, err := modfile.Parse(modPath)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "parse holon.mod: %v", err)
	}

	vendorDir := filepath.Join(dir, ".holon")
	// Clean existing vendor directory
	os.RemoveAll(vendorDir) //nolint:errcheck

	var vendored []*pb.Dependency
	for _, dep := range mod.Require {
		// Skip replaced dependencies
		if mod.ResolvedPath(dep.Path) != "" {
			continue
		}

		src := cachePathFor(dep.Path, dep.Version)
		if _, err := os.Stat(src); os.IsNotExist(err) {
			return nil, status.Errorf(codes.FailedPrecondition,
				"%s@%s not in cache — run 'atlas pull' first", dep.Path, dep.Version)
		}

		// Destination: .holon/<last-path-component>/
		name := filepath.Base(dep.Path)
		dst := filepath.Join(vendorDir, name)

		if err := copyDir(src, dst); err != nil {
			return nil, status.Errorf(codes.Internal, "vendor %s: %v", dep.Path, err)
		}

		vendored = append(vendored, &pb.Dependency{
			Path:      dep.Path,
			Version:   dep.Version,
			CachePath: dst,
		})
	}

	return &pb.VendorResponse{Vendored: vendored}, nil
}

// CleanCache purges the global holon cache directory.
func (s *Server) CleanCache(_ context.Context, _ *pb.CleanCacheRequest) (*pb.CleanCacheResponse, error) {
	cacheDir := CacheDir()
	if err := os.RemoveAll(cacheDir); err != nil {
		return nil, status.Errorf(codes.Internal, "purge cache: %v", err)
	}
	return &pb.CleanCacheResponse{CachePath: cacheDir}, nil
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

// latestCompatibleTag queries remote git tags and returns the latest
// version sharing the same major version (MVS-compatible).
func latestCompatibleTag(depPath, currentVersion string) (string, error) {
	gitURL := "https://" + depPath + ".git"

	cmd := exec.Command("git", "ls-remote", "--tags", "--refs", gitURL)
	out, err := cmd.Output()
	if err != nil {
		// Try without .git suffix
		gitURL = "https://" + depPath
		cmd = exec.Command("git", "ls-remote", "--tags", "--refs", gitURL)
		out, err = cmd.Output()
		if err != nil {
			return "", fmt.Errorf("ls-remote %s: %w", depPath, err)
		}
	}

	currentMajor, _, _, ok := parseSemver(currentVersion)
	if !ok {
		return currentVersion, nil
	}

	// Collect compatible tags (same major version)
	var candidates []string
	for _, line := range strings.Split(string(out), "\n") {
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		ref := parts[1]
		tag := strings.TrimPrefix(ref, "refs/tags/")
		major, _, _, ok := parseSemver(tag)
		if ok && major == currentMajor {
			candidates = append(candidates, tag)
		}
	}

	if len(candidates) == 0 {
		return currentVersion, nil
	}

	sort.Slice(candidates, func(i, j int) bool {
		return compareSemver(candidates[i], candidates[j]) < 0
	})

	return candidates[len(candidates)-1], nil
}

// parseSemver extracts major, minor, patch from "vM.N.P".
func parseSemver(v string) (major, minor, patch int, ok bool) {
	v = strings.TrimPrefix(v, "v")
	parts := strings.SplitN(v, ".", 3)
	if len(parts) != 3 {
		return 0, 0, 0, false
	}
	_, err1 := fmt.Sscan(parts[0], &major)
	_, err2 := fmt.Sscan(parts[1], &minor)
	_, err3 := fmt.Sscan(parts[2], &patch)
	return major, minor, patch, err1 == nil && err2 == nil && err3 == nil
}

// compareSemver returns -1, 0, or 1.
func compareSemver(a, b string) int {
	ma, mia, pa, _ := parseSemver(a)
	mb, mib, pb, _ := parseSemver(b)
	if ma != mb {
		return ma - mb
	}
	if mia != mib {
		return mia - mib
	}
	return pa - pb
}

// copyDir recursively copies src to dst.
func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)

		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()

		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		dstFile, err := os.Create(target)
		if err != nil {
			return err
		}
		defer dstFile.Close()

		_, err = io.Copy(dstFile, srcFile)
		return err
	})
}
