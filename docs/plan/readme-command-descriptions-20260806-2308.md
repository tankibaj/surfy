# README Command Descriptions

**Status:** Done | **Owner:** main-agent | **Updated:** 2026-08-06 23:09

## Goal

Make the README command overview self-explanatory by adding a concise description to every documented invocation.

## Scope

- Update only the README command reference.
- Preserve the existing command syntax and ordering.
- Keep descriptions consistent with `surfy --help`.

## Affected Files

- `README.md`

## Test Strategy

- Documentation-only change: no test-first cycle required.
- Verify Markdown formatting and run the existing test suite before handoff.

## Definition of Done

- [x] Every listed command has a description.
- [x] README remains correctly formatted.
- [x] Existing tests pass (43/43).
