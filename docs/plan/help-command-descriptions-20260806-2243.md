# Help Command Descriptions

**Status:** Done | **Owner:** main-agent | **Updated:** 2026-08-06 22:47

## Goal

Make `surfy --help` a concise command reference with usage syntax and a description for every public command.

## Scope

- Replace the comma-separated command list with aligned command signatures and descriptions.
- Cover the default foreground invocation, project commands, global commands, and certificate subcommands.
- Keep output dependency-free and terminal-friendly.

## Affected Files

- `internal/app/app.go`
- `internal/app/app_test.go`

## Test Strategy

- Add an output-level unit test first.
- Assert representative command signatures, descriptions, and section labels.
- Run formatting, vet, and the full Go test suite through an independent test runner.

## Definition of Done

- [x] Test captures the intended help contract.
- [x] Every public command has a description.
- [x] Formatting, vet, and tests pass.
- [x] Documentation status is `Done`.
