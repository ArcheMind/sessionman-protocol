import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync, readdirSync } from "node:fs";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import Ajv2020 from "ajv/dist/2020.js";
import addFormats from "ajv-formats";
import { materialize, parseSession, upgradeFromV1, validateSession } from "../dist/index.js";

const root = join(dirname(fileURLToPath(import.meta.url)), "..", "..");
const schema = JSON.parse(readFileSync(join(root, "spec", "schema", "unified-transcript.schema.json"), "utf8"));
const ajv = new Ajv2020.default({ strict: false, allErrors: true });
addFormats.default(ajv);
const schemaValidate = ajv.compile(schema);

const fixtures = (kind) =>
  readdirSync(join(root, "spec", "fixtures", kind))
    .filter((name) => name.endsWith(".json"))
    .map((name) => [name, JSON.parse(readFileSync(join(root, "spec", "fixtures", kind, name), "utf8"))]);

for (const [name, session] of fixtures("valid")) {
  test(`valid/${name}: validator accepts`, () => {
    assert.deepEqual(validateSession(session), []);
  });
  test(`valid/${name}: schema accepts`, () => {
    assert.ok(schemaValidate(session), JSON.stringify(schemaValidate.errors));
  });
  test(`valid/${name}: parseSession accepts`, () => {
    const parsed = parseSession(session);
    assert.equal(parsed.protocolVersion, "2.0");
  });
}

for (const [name, wrapper] of fixtures("invalid")) {
  test(`invalid/${name}: states a reason`, () => {
    assert.ok(typeof wrapper.reason === "string" && wrapper.reason.length > 0);
  });
  test(`invalid/${name}: validator rejects`, () => {
    assert.notDeepEqual(validateSession(wrapper.session), []);
  });
  test(`invalid/${name}: schema rejects`, () => {
    assert.equal(schemaValidate(wrapper.session), false);
  });
}

for (const [name, v1] of fixtures("v1")) {
  test(`v1/${name}: upgradeFromV1 and parseSession produce schema-valid 2.0`, () => {
    const upgraded = upgradeFromV1(v1);
    assert.equal(upgraded.protocolVersion, "2.0");
    assert.deepEqual(validateSession(upgraded), []);
    assert.ok(schemaValidate(upgraded), JSON.stringify(schemaValidate.errors));
    const parsed = parseSession(v1);
    assert.deepEqual(parsed, upgraded);
  });
}

test("materialize: drops world×constant, deduplicates slots, applies summary boundary, closes dangling tool_use", () => {
  const full = JSON.parse(readFileSync(join(root, "spec", "fixtures", "valid", "full-20.json"), "utf8"));
  const mat = materialize(full);
  assert.deepEqual(
    mat.map((e) => e.id),
    ["id-1", "board-2", "summary-1", "w-notice-1", "s1-hook-1", "a2"],
  );

  const dangling = {
    protocolVersion: "2.0",
    id: "sess-dangling",
    provider: "claude",
    entries: [
      {
        id: "a1",
        source: "s2",
        mutability: "delta",
        content: [{ type: "tool_use", id: "call-orphan", name: "Bash", input: { cmd: "ls" } }],
      },
    ],
    runtimeEvents: [{ id: "re-1", afterEntryId: "a1", provider: "claude", kind: "turn_aborted" }],
  };
  const matDangling = materialize(dangling);
  assert.equal(matDangling.length, 2);
  assert.equal(matDangling[1].source, "world");
  assert.equal(matDangling[1].mutability, "delta");
  assert.equal(matDangling[1].toolResult?.toolUseId, "call-orphan");
  assert.equal(matDangling[1].toolResult?.isError, true);
});
