import { test } from "node:test";
import assert from "node:assert/strict";
import { interpretStatus, isSupportedFile, matchesPattern, parseAssociations, parseProgressLines, Serializer } from "./core";

test("parseProgressLines: complete events, partial tail preserved, noise skipped", () => {
  const chunk =
    `{"v":1,"phase":"parse","done":1,"total":10}\n` +
    `not json at all\n` +
    `{"v":2,"phase":"future"}\n` +
    `{"v":1,"phase":"resolve","done":3,"total":5}\n` +
    `{"v":1,"phase":"res`; // mid-line split
  const { events, rest } = parseProgressLines(chunk);
  assert.equal(events.length, 2);
  assert.deepEqual(events[0], { v: 1, phase: "parse", done: 1, total: 10 });
  assert.equal(events[1].phase, "resolve");
  assert.equal(rest, `{"v":1,"phase":"res`);
  // feeding the rest plus its completion yields the event
  const round2 = parseProgressLines(rest + `olve","done":5,"total":5}\n`);
  assert.equal(round2.events.length, 1);
  assert.equal(round2.events[0].done, 5);
});

test("interpretStatus: all states, unknown degrades to unindexed", () => {
  assert.equal(interpretStatus({ state: "unindexed" }).kind, "unindexed");
  assert.equal(interpretStatus(null).kind, "unindexed");
  assert.equal(interpretStatus({ state: "???" }).kind, "unindexed");
  const idx = interpretStatus({ state: "indexed", files: 792, symbols: 8991 });
  assert.deepEqual(idx, { kind: "indexed", files: 792, symbols: 8991 });
  const stale = interpretStatus({ state: "stale-schema", schema_version: 6, schema_required: 7 });
  assert.equal(stale.kind, "needs-reindex");
  const building = interpretStatus({ state: "building", phase: "parse", done: 3, total: 10 });
  assert.deepEqual(building, { kind: "building", phase: "parse", done: 3, total: 10 });
  const dead = interpretStatus({ state: "building", stale: true });
  assert.equal(dead.kind, "needs-reindex");
});

test("Serializer: burst collapses to one run; mid-run trigger queues exactly one", async () => {
  let runs = 0;
  let release: () => void = () => {};
  const s = new Serializer(async () => {
    runs++;
    await new Promise<void>((r) => (release = r));
  }, 10);

  s.trigger();
  s.trigger();
  s.trigger(); // burst: one run
  await new Promise((r) => setTimeout(r, 30));
  assert.equal(runs, 1);

  s.trigger(); // arrives while run 1 is still held open
  await new Promise((r) => setTimeout(r, 30));
  assert.equal(runs, 1, "second run must wait for the first");
  release(); // finish run 1 -> queued follow-up fires
  await new Promise((r) => setTimeout(r, 10));
  assert.equal(runs, 2);
  release(); // finish run 2
  await new Promise((r) => setTimeout(r, 30));
  assert.equal(runs, 2, "no phantom third run");
});

test("isSupportedFile", () => {
  assert.ok(isSupportedFile("/x/a.go"));
  assert.ok(isSupportedFile("/x/A.TSX"));
  assert.ok(isSupportedFile("/x/y.php"));
  assert.ok(!isSupportedFile("/x/a.rs"));
  assert.ok(!isSupportedFile("/x/Makefile"));
});

test("parseAssociations: valid, malformed, absent", () => {
  assert.deepEqual(parseAssociations('{"associations":{"*.module":"php","*.inc":"php"}}'), [
    "*.module",
    "*.inc",
  ]);
  assert.deepEqual(parseAssociations('{"associations":{"*.x": 3}}'), []);
  assert.deepEqual(parseAssociations("not json"), []);
  assert.deepEqual(parseAssociations(undefined), []);
});

test("matchesPattern: basename vs path patterns, glob subset", () => {
  assert.ok(matchesPattern("*.module", "web/modules/custom/mymod.module"));
  assert.ok(!matchesPattern("*.module", "a/b/c.php"));
  assert.ok(matchesPattern("legacy/*.tpl", "legacy/page.tpl"));
  assert.ok(!matchesPattern("legacy/*.tpl", "other/page.tpl"));
  assert.ok(matchesPattern("?.inc", "a/x.inc"));
  assert.ok(!matchesPattern("?.inc", "a/xy.inc"));
});

test("isSupportedFile honors associations and new defaults", () => {
  assert.ok(isSupportedFile("a/b.mjs"));
  assert.ok(isSupportedFile("a/b.phtml"));
  assert.ok(isSupportedFile("a/b.pyi"));
  assert.ok(!isSupportedFile("web/mymod.module"));
  assert.ok(isSupportedFile("web/mymod.module", ["*.module"]));
});
