# Unified Transcript Format 2.0

The unified transcript format normalizes agent conversation histories from
multiple providers into a single portable structure. This repository is the
protocol's home: the specification (this document), the machine-readable JSON
Schema, and the reference implementations.

- Normative schema: [`schema/unified-transcript.schema.json`](../schema/unified-transcript.schema.json)
- Go implementation: [`go/`](../../go) (`github.com/ArcheMind/sessionman-protocol/go`)
- TypeScript implementation: [`typescript/src/`](../../typescript/src) (npm package `sessionman-protocol`)
- Shared conformance fixtures: [`fixtures/`](../fixtures)

Consumers (SessionMan, Someone Harness, …) import an implementation from this
repository; they do not define their own transcript shapes. Format changes land
here first — spec + schema + both implementations + fixtures in one commit —
then consumers upgrade their pinned version.

## Design principles

1. **Two orthogonal axes replace `role`.** Every portable message is classified by
   its origin (`source`) and how it evolves over time (`mutability`). Native
   channels, cross-agent restoration, TUI turn boundaries, and deduplication
   derive strictly from these structural fields.
2. **Carrier isolation.** Provider-bound scaffolding (vendor system prompts,
   built-in tool/skill listings, permission modes, environment snapshots) is
   stripped at the import boundary. Provider execution faults (safety refusals,
   aborted turns, quota exhaustion, provider errors) live in top-level
   `runtimeEvents[]` for audit and diagnostics and are **never replayed** into
   model context.
3. **Static world is dropped on restore.** `world × constant` messages (such as
   workspace rule files `AGENTS.md` / `CLAUDE.md`) are dropped when restoring
   into a target agent; the target agent reloads its own workspace rules from
   disk.
4. **No content-specific optimizations.** The protocol and serializers operate
   solely on structure; they never inspect, classify, or rewrite message body
   text.

## Two orthogonal axes

### Axis 1: `source` (who produced it)

| Value | Meaning |
|---|---|
| `s1` | Automatic / subconscious cognition surfaced by the harness (identity, recalled memories, task board, folder view, compaction summaries, waking hooks). |
| `s2` | Conscious model output (reply text, tool invocations, reasoning). Prior models remain `s2` across provider switches. |
| `user` | The human user. Establishes TUI turn boundaries, `session.title`, and user authorization. |
| `world` | Non-human external world: tool execution results (`toolResult`), external async notices (subagent reports, background task completions, chat/webhook notifications), and workspace rule documents. |

### Axis 2: `mutability` (how it evolves)

| Value | Meaning |
|---|---|
| `constant` | Session-invariant premise, keyed by `slot`. Position-independent; latest copy per `(source, slot)` wins. |
| `current` | Mutable state snapshot, keyed by `slot`. Newer messages with the same `(source, slot)` supersede older ones; only the latest copy is materialized. |
| `delta` | Immutable chronological event on the causal chain. Order matters; all messages are preserved. |

### The 4×3 matrix & cross-agent restore rules

| `source` | `constant` | `current` | `delta` |
|---|---|---|---|
| **`s1`** | Restore latest per `slot` (pinned prefix, harness channel) | Restore latest per `slot` (harness channel) | Restore in order (harness channel) |
| **`s2`** | *(forbidden)* | *(forbidden)* | Restore in order (`assistant`; foreign thinking per target provider policy) |
| **`user`** | Restore latest per `slot` (harness channel) | Restore latest per `slot` (harness channel) | Restore in order (`user` / `USER_INPUT`) |
| **`world`** | **Drop on restore** | Restore latest per `slot` (harness channel) | `toolResult` → tool result channel; `content` → non-user external channel |

## Session envelope

```json
{
  "protocolVersion": "2.0",
  "id": "string",
  "provider": "string (e.g. claude | codex | agy | pi | opencode)",
  "isSubagent": false,
  "workspace": "/absolute/path",
  "title": "first human intent (≤96 chars)",
  "startedAt": "RFC3339Nano UTC",
  "updatedAt": "RFC3339Nano UTC",
  "messageCount": 0,
  "source": "native file path",
  "messages": [ "...Message[]" ],
  "runtimeEvents": [ "...RuntimeEvent[] (optional)" ]
}
```

## Message

```json
{
  "id": "string",
  "parentId": "string | null",
  "source": "s1 | s2 | user | world",
  "mutability": "constant | current | delta",
  "slot": "string (required for constant/current; forbidden for delta)",
  "hash": "string (optional for constant/current; forbidden for delta)",
  "supersedesBeforeId": "string (optional for current; forbidden for constant/delta)",
  "content": [ "...ContentBlock[] (forbidden when toolResult is present)" ],
  "toolResult": {
    "toolUseId": "string",
    "toolName": "string",
    "result": "any JSON value (optional, verbatim)",
    "isError": false
  },
  "producer": {
    "provider": "string (optional)",
    "model": "string (optional)",
    "origin": "string (optional)"
  },
  "stopReason": "string (optional, s2 delta only)",
  "timestamp": "RFC3339Nano UTC (event time)",
  "recordedAt": "RFC3339Nano UTC (optional; record write time when different)"
}
```

### Structural invariants

- `mutability: "constant"` or `"current"` requires `slot` and `content`, and
  forbids `toolResult` and `stopReason`. `supersedesBeforeId` is allowed only on
  `current`.
- `mutability: "delta"` forbids `slot`, `hash`, and `supersedesBeforeId`.
- `source: "s1"` requires `content` with `text` blocks only, and forbids
  `toolResult` and `stopReason`.
- `source: "s2"` requires `mutability: "delta"` and `content` (`text`,
  `thinking`, `tool_use`), and forbids `image` blocks and `toolResult`.
- `source: "user"` requires `content` (`text`, `image`), and forbids
  `toolResult` and `stopReason`.
- `source: "world"` forbids `stopReason`, and carries **either** `toolResult`
  (only when `mutability: "delta"`, with `content` absent) **or** `content`
  (`text`, `image`, with `toolResult` absent).

### Tool result

`toolResult.result` is any JSON value carried verbatim from the provider. Absent
means empty output. When a serializer writes a `toolResult` message into a
destination's native format, it writes `result` verbatim if the destination
accepts that shape; otherwise it writes the text projection (for a block array,
`text` fields joined with `\n`; for any other non-string value, compact JSON).
Native shapes per destination:

- Claude Code: `tool_result.content` accepts a string or an array of `text`/`image` blocks.
- Codex CLI: `output` accepts a string or Codex content-item array.
- Pi: `content` accepts an array of `text`/`image` blocks (a string wraps into one `text` block).

### Causal chain

`parentId` links `delta` messages to their causal predecessor. The first delta of
a session has `parentId: null` (or omitted). `constant` and `current` messages
are position-independent snapshots keyed by `(source, slot)`.

## ContentBlock

Content is an ordered array of typed blocks discriminated by `type`.

### text

```json
{ "type": "text", "text": "string" }
```

### image

Allowed in `user` and `world` messages with `content`. `source` is a free-form
object carried verbatim from the provider.

```json
{ "type": "image", "source": { "type": "base64", "media_type": "image/png", "data": "..." } }
```

### thinking

Allowed only in `s2 × delta`. `id` is the provider's reasoning item id (Codex);
`signature` is the provider's integrity/encryption proof. Both must survive
same-provider round trips verbatim.

```json
{
  "type": "thinking",
  "id": "string (optional, provider reasoning item id)",
  "text": "string (empty if encrypted)",
  "signature": "string (optional, provider-specific integrity proof)"
}
```

### tool_use

Allowed only in `s2 × delta`.

```json
{
  "type": "tool_use",
  "id": "string (unique, referenced by toolResult.toolUseId)",
  "name": "string",
  "input": {}
}
```

## RuntimeEvent (carrier boundary)

Execution-carrier incidents are recorded outside `messages[]` so diagnostics and
UIs retain full visibility while cross-agent replay never feeds provider errors
or safety refusals back into model context.

```json
{
  "id": "string",
  "afterMessageId": "string (optional)",
  "provider": "string",
  "model": "string (optional)",
  "kind": "safety_refusal | turn_aborted | quota_exhausted | provider_error",
  "code": "string (optional, e.g. reasoning_extraction)",
  "message": "string (optional)",
  "timestamp": "RFC3339Nano UTC (optional)"
}
```

## Deterministic materialization

Before serializing a 2.0 session for a target agent, `Materialize(session)`
projects `messages[]` deterministically:

1. **Drop `world × constant`** (target agent loads workspace rules from disk).
2. **Deduplicate `constant`** by `(source, slot)`, keeping the last message in
   input order, placed first (stable prefix for prompt caching).
3. **Deduplicate `current`** by `(source, slot)`, keeping the last message in
   input order. If a surviving `current` message declares `supersedesBeforeId`,
   delta messages strictly before that message id are omitted and the summary is
   placed at the cut boundary. Remaining `current` messages are placed after
   constants (or before the final user/world turn to preserve prefix cache).
4. **Preserve `delta`** messages in causal order.
5. **Close dangling `tool_use` calls.** Any `s2 × delta` `tool_use` without a
   matching `world × delta` `toolResult` (e.g. after a `turn_aborted` or
   `safety_refusal` runtime event) receives a synthetic interrupted `toolResult`
   (`isError: true`) at serialization time.
6. **Ignore `runtimeEvents[]`.**

## Target serializer channels

Channel selection follows verified model-visibility rules (Claude Code's
`system/informational` records and all providers' unsigned native thinking
fields are invisible to the model):

| 2.0 Message | Claude Code | Codex CLI | AGY | Pi |
|---|---|---|---|---|
| `s1` (`constant` / `current` / `delta`) | `user(isMeta: true)` | `developer` | `SYSTEM_MESSAGE` | `system` |
| `user` (`constant` / `current`) | `user(isMeta: true)` | `developer` | `SYSTEM_MESSAGE` | `system` |
| `user × delta` | `user` | `user` | `USER_INPUT` | `user` |
| `world × constant` | *(dropped)* | *(dropped)* | *(dropped)* | *(dropped)* |
| `world × current` | `user(isMeta: true)` | `developer` | `SYSTEM_MESSAGE` | `system` |
| `world × delta` (`content`) | `user(isMeta: true)` | `developer` / non-human `user` | `SYSTEM_MESSAGE` | `system` |
| `world × delta` (`toolResult`) | `user` (`tool_result`) | `function_call_output` / `custom_tool_call_output` | tool step | `toolResult` |
| `s2 × delta` (`text` / `tool_use`) | `assistant` | `assistant` + `function_call` | `PLANNER_RESPONSE` | `assistant` |
| `s2 × delta` (`thinking`, same provider/model + signature) | native `thinking` | native `reasoning` + `encrypted_content` | native `thinking_signature` | native `thinking` |
| `s2 × delta` (`thinking`, foreign / unsigned) | omitted (or `user(isMeta: true)`) | `developer` | `SYSTEM_MESSAGE` | `system` |

### Foreign thinking replay

- Foreign or unsigned `thinking` blocks must **not** be written into `assistant`
  text and must **not** wrap visible text in explicit reasoning tags (such as
  `<think>` or `<prior-reasoning>`).
- **Provider safety note (verified 2026-11-27 on Claude Opus 5.5):** A/B replay
  of a refused production session showed that even unlabelled foreign reasoning
  placed in `user(isMeta: true)` can still trigger Claude's
  `[reasoning_extraction]` safety classifier, whereas omitting foreign thinking
  while keeping `s1` messages in `user(isMeta: true)` and dropping `runtimeEvents`
  eliminated the classifier stop completely. Accordingly, canonical transcripts
  always preserve `thinking` blocks verbatim in `s2 × delta`, while target
  serializers project foreign unsigned thinking into the target's harness
  channel or omit it according to target provider policy (for Claude, omit by
  default). Native session files are treated as lossy provider projections;
  there is no separate out-of-band mapping layer.

## Provider import rules

Parsers classify native records by origin rather than blindly copying native
roles:

- **Claude Code**: `user` (normal) → `user × delta`; `user(isMeta)` → `s1 × delta`
  or `world × delta`; `user(isCompactSummary)`
  → `s1 × current` (`slot: "compaction-summary"`); `user(tool_result)` → `world × delta`
  (`toolResult`); `assistant` → `s2 × delta`; `attachment.relevant_memories` →
  `s1 × current`; `attachment.instructions` → `world × constant`;
  `attachment.{environment,model,skill_listing,auto_mode,...}` → stripped
  (carrier); synthetic safety refusal / `model_refusal` → `runtimeEvents[]`.
- **Codex CLI**: `response_item` `message(user)` → `user × delta` (except
  `<environment_context>` which is stripped as carrier, `# AGENTS.md instructions`
  → `world × constant`, and `<subagent_notification>` → `world × delta`);
  `developer` carrier records (vendor prompt, `<tools>`, `<permissions>`) →
  stripped; `developer` `<turn_aborted>` → `runtimeEvents[]`; other `developer`
  → `s1`; `compacted` → `s1 × current` (`slot: "compaction-summary"`);
  `assistant` / `reasoning` / `function_call` / `custom_tool_call` → `s2 × delta`;
  `function_call_output` / `custom_tool_call_output` → `world × delta` (`toolResult`).
- **AGY**: `USER_INPUT` → `user × delta`; `PLANNER_RESPONSE` → `s2 × delta`
  (interleaved tool steps → `world × delta` `toolResult`); `SYSTEM_MESSAGE` →
  `s1` / `world × delta`; `CHECKPOINT` →
  `s1 × current` (`slot: "compaction-summary"`); `DIRECTORY_RULES` →
  `world × constant`; `RETRIEVE_MEMORY` → `s1 × current`; `ERROR_MESSAGE` →
  `runtimeEvents[]`.
- **Pi**: `user` → `user × delta`; `assistant` → `s2 × delta`; `toolResult` →
  `world × delta` (`toolResult`); `system` → `s1 × delta`.

## v1 → v2 structural compatibility

Both SDKs expose `UpgradeFromV1` / `upgradeFromV1` and `ParseSession` /
`parseSession` to upgrade v0.x/v1 (`messages[]` with `role`) sessions without
content inspection:

| v1 `Message.role` | v2 `Message` |
|---|---|
| `user` | `source: "user"`, `mutability: "delta"`, `content` preserved |
| `tool` | `source: "world"`, `mutability: "delta"`, `toolResult: { toolUseId, toolName, result, isError }` |
| `assistant` | `source: "s2"`, `mutability: "delta"`, `content` preserved, `producer: { provider: session.provider, model: msg.model }`, `stopReason` preserved |
| `system` | `source: "s1"`, `mutability: "delta"`, `content` preserved |

## Offload

When an implementation offloads older turns to disk to save model context, it
writes the offloaded slice as a canonical 2.0 session in **YAML**
(`offload-*.yaml`) rather than JSON to save tokens and preserve readable
multiline text. Loaders must remain backward-compatible with legacy JSON offload
files (`offload-*.json`, v1 or 2.0).

## Validation

The normative constraints live in
[`schema/unified-transcript.schema.json`](../schema/unified-transcript.schema.json).
Both implementations expose validators enforcing every schema rule plus
message-id uniqueness and runtime-event-id uniqueness within a session.
Cross-message referential checks (`parentId` / `toolUseId` / `supersedesBeforeId`
resolution) are intentionally not enforced: native sessions may be truncated
mid-chain.
