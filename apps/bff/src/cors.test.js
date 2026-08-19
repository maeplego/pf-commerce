import assert from "node:assert/strict";
import { test } from "node:test";
import { graphqlOriginGate } from "./cors.js";

test("missing Origin is allowed for curl and server-side fetch", () => {
  const gate = graphqlOriginGate("", "http://localhost:3009");
  assert.equal(gate.ok, true);
  assert.equal(gate.headers["Access-Control-Allow-Origin"], undefined);
});

test("storefront origin is echoed", () => {
  const gate = graphqlOriginGate("http://localhost:3009", "http://localhost:3009");
  assert.equal(gate.ok, true);
  assert.equal(gate.headers["Access-Control-Allow-Origin"], "http://localhost:3009");
});

test("other browser origins are rejected", () => {
  const gate = graphqlOriginGate("https://evil.example", "http://localhost:3009");
  assert.equal(gate.ok, false);
});

test("star remains explicit opt-in", () => {
  const gate = graphqlOriginGate("https://evil.example", "*");
  assert.equal(gate.ok, true);
  assert.equal(gate.headers["Access-Control-Allow-Origin"], "*");
});
