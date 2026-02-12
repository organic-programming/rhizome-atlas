# Rhizome Atlas

> *"Know what you need."*

Atlas is the holon dependency manager. It resolves, fetches, caches, and
verifies holon dependencies declared in `holon.mod` and `holon.sum`.

Named after the rhizome — a non-hierarchical root network where any node
can connect to any other — because holon dependencies form a graph, not a tree.

## Commands

```
atlas init <holon-path>        — create holon.mod in current directory
atlas add <path> <version>     — add a dependency
atlas remove <path>            — remove a dependency
atlas pull                     — fetch all dependencies to cache
atlas update                   — update deps to latest compatible version
atlas verify                   — check holon.sum integrity
atlas graph                    — display dependency tree
atlas vendor                   — copy cached deps to local .holon/
atlas cache clean              — purge the global cache
atlas serve [--listen <URI>]   — start gRPC server
```

## Facets

| Facet | Access | Example |
|-------|--------|---------|
| **CLI** | Direct invocation | `atlas add github.com/org/dep v0.1.0` |
| **gRPC** | Via OP or any client | `op grpc+stdio://atlas Add '{...}'` |
| **API** | Go import | `import "rhizome-atlas/pkg/modfile"` |

## Organic Programming

This holon is part of the [Organic Programming](https://github.com/Organic-Programming/seed)
ecosystem. For context, see:

- [Constitution](https://github.com/Organic-Programming/seed/blob/master/AGENT.md) — what a holon is
- [Methodology](https://github.com/Organic-Programming/seed/blob/master/METHODOLOGY.md) — how to develop with holons
- [Terminology](https://github.com/Organic-Programming/seed/blob/master/TERMINOLOGY.md) — glossary of all terms
- [Dependencies](https://github.com/Organic-Programming/seed/blob/master/DEPENDENCIES.md) — holon.mod & holon.sum specification

