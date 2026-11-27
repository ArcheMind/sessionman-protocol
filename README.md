# sessionman-protocol

Shared formats and portable integration contracts for
[SessionMan](https://github.com/ArcheMind/sessionman) and downstream consumers.

- **Integration principles**: [spec/docs/unified-integration-principles.md](spec/docs/unified-integration-principles.md)
- **Transcript spec (2.0)**: [spec/docs/unified-transcript-format.md](spec/docs/unified-transcript-format.md)
- **Hook lifecycle spec**: [spec/docs/unified-hook-lifecycle.md](spec/docs/unified-hook-lifecycle.md)
- **Hook configuration spec**: [spec/docs/unified-hook-configuration.md](spec/docs/unified-hook-configuration.md)
- **Hook implementation example**: [spec/docs/unified-hook-implementation-example.md](spec/docs/unified-hook-implementation-example.md)
- **Normative schema**: [spec/schema/unified-transcript.schema.json](spec/schema/unified-transcript.schema.json)
- **Go**: `github.com/ArcheMind/sessionman-protocol/go`
- **TypeScript**: npm package `sessionman-protocol` (install from git, pinned to a commit)
- **Fixtures**: [spec/fixtures/](spec/fixtures/) — shared conformance corpus; both
  implementations must accept every `valid/` session, reject every `invalid/`
  session (`invalid/` files wrap the session with a `reason`), and upgrade
  every `v1/` session cleanly via `UpgradeFromV1` / `upgradeFromV1`

## Rules

Consumers import an implementation from this repository; they never define
their own transcript message shapes. Format changes land here first — spec +
schema + both implementations + fixtures in one commit — then consumers
upgrade their pinned version.

## Develop

```
make test   # go test + tsc + node --test
```
