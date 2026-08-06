# Fix macOS State Ownership

**Status:** Done | **Owner:** main-agent | **Updated:** 2026-08-06

## Goal

Ensure the privileged edge installer leaves the per-user state directory and registry files owned by the invoking user.

## Scope

- Create and validate state as the unprivileged user before any installer elevation.
- Add tag-driven CI release automation and Homebrew formula updates.
- Cover path validation and ownership behavior with tests.
- Document the fix and verify the full suite.

## Affected Files

- `internal/app/app.go`
- `internal/app/app_test.go`, `.github/workflows/release.yml`
- `README.md`

## Test Strategy

- Test rejection of unsafe state paths and symlinks.
- Test ownership preparation against an isolated temporary Application Support path.
- Run the full Go suite and Apple Silicon build separately.

## Definition of Done

- [x] Installer prepares user-owned state safely.
- [x] Existing root-owned state is detected with safe recovery guidance.
- [x] Tests pass.
- [x] Apple Silicon build passes.
