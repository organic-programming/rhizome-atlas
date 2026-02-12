# Rhizome Atlas

> *"Know what you need."*

Atlas is the holon dependency manager. It resolves, fetches, caches, and
verifies holon dependencies declared in `holon.mod` and `holon.sum`.

Named after the rhizome — a non-hierarchical root network where any node
can connect to any other — because holon dependencies form a graph, not a tree.

## Commands

```
atlas init                     — create holon.mod in current directory
atlas add <path> <version>     — add a dependency
atlas pull                     — fetch all dependencies to cache
atlas verify                   — check holon.sum integrity
atlas graph                    — display dependency tree
atlas vendor                   — copy cached deps to local .holon/
```

## Status

Identity defined. Code not yet implemented.

## Organic Programming

This holon is part of the [Organic Programming](https://github.com/Organic-Programming/seed)
ecosystem. For context, see:

- [Constitution](https://github.com/Organic-Programming/seed/blob/master/AGENT.md) — what a holon is
- [Methodology](https://github.com/Organic-Programming/seed/blob/master/METHODOLOGY.md) — how to develop with holons
- [Terminology](https://github.com/Organic-Programming/seed/blob/master/TERMINOLOGY.md) — glossary of all terms
- [Dependencies](https://github.com/Organic-Programming/seed/blob/master/DEPENDENCIES.md) — holon.mod & holon.sum specification
