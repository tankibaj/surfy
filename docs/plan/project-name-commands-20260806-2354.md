# Project Name Commands

**Status:** Done | **Owner:** main-agent | **Updated:** 2026-08-06 23:58

## Goal

Make every project command accept a registered public project name as well as a filesystem path.

## Scope

- Central name-or-path manifest resolution.
- Apply it to `start`, `stop`, `restart`, `status`, `open`, `enable`, and `disable`.
- Preserve current-directory and explicit-path behavior.

## Affected Files

- `internal/app/app.go`
- `internal/app/app_test.go`
- `README.md`

## Test Strategy

- [x] Resolve a registered name to its canonical manifest.
- [x] Prefer an existing filesystem path when supplied.
- [x] Verify `status` and `stop` no longer emit missing-manifest errors for a project name.
- [x] Run the full suite.

## Definition of Done

- [x] `serve-them stop deliverydeck` targets the registered project.
- [x] All project commands share the same selector behavior.
- [x] Tests pass (26/26).
