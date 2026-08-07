# Fix macOS State Ownership

**Status:** Done | **Owner:** main-agent | **Updated:** 2026-08-06 23:08

## Goal

Ensure `surfy install` securely migrates legacy root-owned default macOS state to the invoking user without manual recovery commands.

## Scope

- Keep normal and custom state-directory preparation unprivileged.
- On a failed default-directory preparation, run a narrowly scoped privileged migration in the existing sudo flow.
- Derive the only privileged target from the validated invoking UID/GID and account home; never accept or recursively traverse a caller-controlled target.
- Reject symlinked state paths or known registry artifacts.
- Bind ownership changes to validated open file descriptors, reject multiply linked files and inode replacement, and leave the edge socket to the edge service.
- Cover identity, path, symlink, non-recursive ownership, and custom-directory behavior with tests.

## Affected Files

- `internal/app/app.go`
- `internal/app/app_test.go`
- `docs/plan/fix-state-ownership-20260806-2200.md`

## Test Strategy

- Test rejection of missing, malformed, root, and account-mismatched sudo identity.
- Test canonical target derivation and rejection of symlinked paths/artifacts.
- Assert migration changes ownership only on the state directory and known direct registry files, never nested content.
- Test hard-link and checked-to-opened inode replacement rejection.
- Test that custom state directories are prepared without privileged migration and fail closed when inaccessible.
- Run focused app tests and vet, then cross-compile the app tests for the release target.

## Definition of Done

- [x] Installer prepares normal and custom user-owned state safely.
- [x] Existing root-owned default state is migrated automatically through a validated privileged entry point.
- [x] No recursive ownership change can target caller-controlled paths.
- [x] Ownership changes are fd-bound and reject hard links or replaced inodes.
- [x] Focused tests, vet, and Darwin ARM64 test compilation pass.
