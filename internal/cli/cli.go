// Package cli implements the CLI facet of Rhizome Atlas.
// Each subcommand delegates to the gRPC service implementation so that
// CLI and gRPC facets share the same logic.
package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/Organic-Programming/rhizome-atlas/internal/server"
	pb "github.com/Organic-Programming/rhizome-atlas/proto"
)

// Run executes the CLI with the given arguments.
func Run(args []string) int {
	if len(args) == 0 {
		printUsage()
		return 1
	}

	srv := &server.Server{}
	ctx := context.Background()

	switch args[0] {
	case "init":
		return cmdInit(ctx, srv, args[1:])
	case "add":
		return cmdAdd(ctx, srv, args[1:])
	case "remove":
		return cmdRemove(ctx, srv, args[1:])
	case "pull":
		return cmdPull(ctx, srv, args[1:])
	case "verify":
		return cmdVerify(ctx, srv, args[1:])
	case "graph":
		return cmdGraph(ctx, srv, args[1:])
	case "help", "--help", "-h":
		printUsage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "atlas: unknown command %q\n", args[0])
		printUsage()
		return 1
	}
}

func cmdInit(ctx context.Context, srv *server.Server, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: atlas init <holon-path>")
		return 1
	}

	resp, err := srv.Init(ctx, &pb.InitRequest{
		Directory: ".",
		HolonPath: args[0],
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "atlas init: %v\n", err)
		return 1
	}
	fmt.Printf("created %s\n", resp.ModFile)
	return 0
}

func cmdAdd(ctx context.Context, srv *server.Server, args []string) int {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: atlas add <path> <version>")
		return 1
	}

	resp, err := srv.Add(ctx, &pb.AddRequest{
		Directory: ".",
		Path:      args[0],
		Version:   args[1],
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "atlas add: %v\n", err)
		return 1
	}
	dep := resp.Dependency
	if dep.CachePath != "" {
		fmt.Printf("added %s@%s → %s\n", dep.Path, dep.Version, dep.CachePath)
	} else {
		fmt.Printf("added %s@%s (fetch deferred)\n", dep.Path, dep.Version)
	}
	return 0
}

func cmdRemove(ctx context.Context, srv *server.Server, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: atlas remove <path>")
		return 1
	}

	_, err := srv.Remove(ctx, &pb.RemoveRequest{
		Directory: ".",
		Path:      args[0],
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "atlas remove: %v\n", err)
		return 1
	}
	fmt.Printf("removed %s\n", args[0])
	return 0
}

func cmdPull(ctx context.Context, srv *server.Server, _ []string) int {
	resp, err := srv.Pull(ctx, &pb.PullRequest{Directory: "."})
	if err != nil {
		fmt.Fprintf(os.Stderr, "atlas pull: %v\n", err)
		return 1
	}
	for _, dep := range resp.Fetched {
		fmt.Printf("  %s@%s → %s\n", dep.Path, dep.Version, dep.CachePath)
	}
	if len(resp.Fetched) == 0 {
		fmt.Println("all dependencies up to date")
	}
	return 0
}

func cmdVerify(ctx context.Context, srv *server.Server, _ []string) int {
	resp, err := srv.Verify(ctx, &pb.VerifyRequest{Directory: "."})
	if err != nil {
		fmt.Fprintf(os.Stderr, "atlas verify: %v\n", err)
		return 1
	}
	if resp.Ok {
		fmt.Println("all verified")
		return 0
	}
	for _, e := range resp.Errors {
		fmt.Fprintf(os.Stderr, "  %s\n", e)
	}
	return 1
}

func cmdGraph(ctx context.Context, srv *server.Server, _ []string) int {
	resp, err := srv.Graph(ctx, &pb.GraphRequest{Directory: "."})
	if err != nil {
		fmt.Fprintf(os.Stderr, "atlas graph: %v\n", err)
		return 1
	}

	fmt.Println(resp.Root)
	for _, edge := range resp.Edges {
		fmt.Printf("  %s → %s@%s\n", edge.From, edge.To, edge.Version)
	}
	return 0
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `Rhizome Atlas — holon dependency manager

Usage:
  atlas <command> [arguments]

Commands:
  init <holon-path>            create holon.mod in current directory
  add <path> <version>         add a dependency
  remove <path>                remove a dependency
  pull                         fetch all dependencies to cache
  verify                       check holon.sum integrity
  graph                        display dependency tree
  serve [--listen <URI>]       start gRPC server

`)
}
