---
# Holon Identity v1
uuid: "00000000-0000-4000-0000-000000000002"
given_name: "Rhizome"
family_name: "Atlas"
motto: "Know what you need."
composer: "B. ALTER"
clade: "deterministic/stateful"
status: draft
born: "2026-02-12"

# Lineage
parents: ["00000000-0000-4000-0000-000000000001"]
reproduction: "manual"

# Pinning
binary_path: null
binary_version: "0.1.0"
git_tag: null
git_commit: null
os: "darwin"
arch: "arm64"
dependencies: []

# Optional
aliases: ["atlas", "rhizome"]
wrapped_license: null

# Metadata
generated_by: "manual"
lang: "go"
proto_status: draft
---

# Rhizome Atlas

> *"Know what you need."*

## Description

Rhizome Atlas is the dependency manager for holons. She resolves, fetches,
caches, and verifies holon dependencies declared in `holon.mod`.

She is named after the rhizome — a non-hierarchical root network where
any node can connect to any other — because holon dependencies form a
graph, not a tree.

Sophia gives holons their identity. Rhizome Atlas gives them their
dependencies. A holon without Sophia has no name; a holon without
Rhizome Atlas has no roots.

## Commands

```
atlas init                     — create holon.mod in current directory
atlas add <path> <version>     — add a dependency
atlas remove <path>            — remove a dependency
atlas pull                     — fetch all dependencies to cache
atlas update                   — update dependencies to latest compatible
atlas verify                   — check holon.sum integrity
atlas graph                    — display dependency tree
atlas vendor                   — copy cached deps to local .holon/
atlas cache clean              — purge the global cache
```

## Contract

- Proto file: `rhizome_atlas.proto`
- Service: `RhizomeAtlasService`
- RPCs: `Init`, `AddDependency`, `RemoveDependency`, `Pull`, `Update`,
  `Verify`, `Graph`, `Vendor`, `CleanCache`

## Files Managed

| File | Purpose |
|------|---------|
| `holon.mod` | Dependency manifest — what this holon needs |
| `holon.sum` | Integrity hashes — proof that deps haven't been tampered with |
| `~/.holon/cache/` | Global machine cache — shared across projects |
| `.holon/` | Optional local vendor directory |
