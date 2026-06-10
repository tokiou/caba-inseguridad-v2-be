# OpenSpec

This project uses [OpenSpec](https://openspec.dev/) (spec-driven development) instead of the former
`docs/sdd/` flow. **Specs are agreed before code is written.**

## Layout

```
openspec/
├── project.md            # shared project context (stack, architecture, conventions)
├── specs/                # SOURCE OF TRUTH — current, implemented behavior, one dir per capability
│   └── <capability>/spec.md
└── changes/              # proposed changes, one folder per change
    └── archive/          # completed changes, merged back into specs/
```

## Spec file format

```markdown
# <Capability> Specification

## Purpose
<what this capability is for>

## Requirements

### Requirement: <Name>
The system SHALL/MUST/SHOULD <behavior statement>.

#### Scenario: <Name>
- GIVEN <initial condition>
- WHEN <action>
- THEN <expected outcome>
- AND <additional outcome>
```

## Workflow for a new feature or change

1. **Propose** — create `changes/<change-name>/` with:
   - `proposal.md` — why, what, scope (in/out).
   - `tasks.md` — ordered implementation tasks.
   - `design.md` — technical decisions / trade-offs (when non-trivial).
   - `specs/<capability>/spec.md` — **delta spec** marking `## ADDED Requirements`,
     `## MODIFIED Requirements`, and/or `## REMOVED Requirements`.
2. **Review & agree** on the proposal before implementing.
3. **Implement** the tasks; keep `go build ./...` and `go test ./...` green.
4. **Archive** — merge the change's delta into `specs/`, then move the folder to
   `changes/archive/<YYYY-MM-DD>-<change-name>/` so `specs/` always reflects current state.

The capability specs under `specs/` were migrated from the previous `docs/sdd/` documents and
describe the system as currently implemented.
