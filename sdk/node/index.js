// codeindex runtime-evidence SDK for Node: in-process CPU sampling via the
// built-in inspector (V8 sampler — no dependencies), spooled as cxprof v1
// into <repo>/.codeindex/runtime/ where the next codeindex query ingests it.
//
// Non-disruption contract: sampling only; failures are swallowed and
// counted (agent.dropped); CODEINDEX_PROFILING=off disables everything;
// frames-only payloads; atomic temp+rename spool writes.
//
// Usage:
//   const agent = require('@codeindex/agent');
//   const stop = agent.start({ repo: '/path/to/repo' });
//   // ... app runs ...
//   stop(); // or rely on the automatic beforeExit flush
'use strict';

const inspector = require('node:inspector');
const fs = require('node:fs');
const path = require('node:path');
const { fileURLToPath } = require('node:url');

const agent = { dropped: 0 };

function start(opts = {}) {
  if (process.env.CODEINDEX_PROFILING === 'off') return () => {};

  const repo = opts.repo || process.env.CODEINDEX_REPO || process.cwd();
  const hz = opts.hz > 0 ? opts.hz : 99;
  const tag = opts.tag || 'dev';
  const startTs = Math.floor(Date.now() / 1000);

  let session;
  try {
    session = new inspector.Session();
    session.connect();
    session.post('Profiler.enable');
    session.post('Profiler.setSamplingInterval', { interval: Math.floor(1e6 / hz) });
    session.post('Profiler.start');
  } catch {
    agent.dropped++;
    return () => {};
  }

  let stopped = false;
  const stop = () => {
    if (stopped) return;
    stopped = true;
    try {
      session.post('Profiler.stop', (err, { profile } = {}) => {
        try {
          if (!err && profile) writeSpool(repo, hz, tag, opts.commit, startTs, profile);
          else agent.dropped++;
        } catch {
          agent.dropped++;
        }
        try { session.disconnect(); } catch { /* already gone */ }
      });
    } catch {
      agent.dropped++;
    }
  };
  process.once('beforeExit', stop);
  return stop;
}

// Convert a V8 .cpuprofile (node tree + per-sample node ids) to cxprof.
function writeSpool(repo, hz, tag, commit, startTs, profile) {
  const byId = new Map();
  for (const n of profile.nodes) byId.set(n.id, n);

  // Stack per node id: walk parents, emit [file, line] outermost-first
  // (cxprof wants innermost LAST, and the parent chain is innermost-first,
  // so build reversed).
  const stackCache = new Map();
  const frameOf = (n) => {
    const cf = n.callFrame;
    if (!cf || !cf.url || cf.lineNumber == null || cf.lineNumber < 0) return null;
    let file = cf.url;
    if (file.startsWith('file://')) {
      try { file = fileURLToPath(file); } catch { return null; }
    }
    if (file.startsWith('node:')) return null; // runtime internals
    return [file, cf.lineNumber + 1]; // V8 lines are 0-based
  };
  const parents = new Map();
  for (const n of profile.nodes) {
    for (const c of n.children || []) parents.set(c, n.id);
  }
  const stackFor = (id) => {
    if (stackCache.has(id)) return stackCache.get(id);
    const frames = [];
    let cur = id;
    while (cur != null) {
      const n = byId.get(cur);
      if (!n) break;
      const f = frameOf(n);
      if (f) frames.push(f);
      cur = parents.get(cur);
    }
    frames.reverse(); // now innermost LAST
    stackCache.set(id, frames);
    return frames;
  };

  const counts = new Map();
  for (const id of profile.samples || []) {
    const frames = stackFor(id);
    if (!frames.length) continue;
    const key = JSON.stringify(frames);
    counts.set(key, (counts.get(key) || 0) + 1);
  }
  if (!counts.size) { agent.dropped++; return; }

  const head = {
    cxprof: 1, lang: 'node', unit: 'samples', hz,
    start: startTs, end: Math.floor(Date.now() / 1000), tag,
  };
  if (commit) head.commit = commit;
  const lines = [JSON.stringify(head)];
  for (const key of [...counts.keys()].sort()) {
    lines.push(JSON.stringify({ st: JSON.parse(key), n: counts.get(key) }));
  }

  const dir = path.join(repo, '.codeindex', 'runtime');
  fs.mkdirSync(dir, { recursive: true });
  const final = path.join(dir, `${startTs}-${process.pid}.cxprof.jsonl`);
  fs.writeFileSync(final + '.tmp', lines.join('\n') + '\n');
  fs.renameSync(final + '.tmp', final);
}

module.exports = { start, agent };
