"""node-registry: string-keyed registry dispatch in Node, profiled with the
zero-dep sdk/node sampler. Static analysis cannot connect dispatchEvent to
handleOrderShipped (the wiring is a runtime string key); the observed edge
from the profile is the only evidence — exactly what the pipeline must carry
end to end."""

import os
import shutil

from . import Scenario, StepError, cluster_blocks

QUERY = "how are shipped order events handled"
BASE_WORK_MS = 1500

REGISTRY_JS = """'use strict';
// String-keyed event registry: static analysis cannot see dispatch targets.
const handlers = {};

function registerHandler(eventName, fn) {
  handlers[eventName] = fn;
}

function dispatchEvent(eventName, payload) {
  const fn = handlers[eventName];
  if (!fn) throw new Error('no handler registered for ' + eventName);
  return fn(payload);
}

module.exports = { registerHandler, dispatchEvent };
"""

HANDLERS_JS = """'use strict';
// Handler for shipped-order events; real CPU work so the sampler sees it.
function handleOrderShipped(order) {
  const deadline = Date.now() + (order.workMs || 1500);
  let acc = 0;
  while (Date.now() < deadline) {
    for (let i = 1; i < 20000; i++) {
      acc += Math.sqrt(i * (order.id + 1)) % 7;
    }
  }
  return acc;
}

module.exports = { handleOrderShipped };
"""

MAIN_JS = """'use strict';
const path = require('node:path');
const fs = require('node:fs');
const agent = require(process.env.SELFHEAL_SDK);
const { registerHandler, dispatchEvent } = require('./registry');
const { handleOrderShipped } = require('./handlers');

const repo = process.env.SELFHEAL_REPO || __dirname;
const workMs = parseInt(process.env.SELFHEAL_WORK_MS || '1500', 10);

const stop = agent.start({ repo, hz: 199 });
registerHandler('order.shipped', handleOrderShipped);
dispatchEvent('order.shipped', { id: 7, workMs });
stop();

// The spool flush is async (Profiler.stop callback); wait for it.
const spool = path.join(repo, '.codeindex', 'runtime');
const t0 = Date.now();
const iv = setInterval(() => {
  let done = false;
  try {
    done = fs.readdirSync(spool).some((f) => f.endsWith('.cxprof.jsonl'));
  } catch {}
  if (done || Date.now() - t0 > 10000) {
    clearInterval(iv);
    process.exit(done ? 0 : 3);
  }
}, 50);
"""


class NodeRegistryScenario(Scenario):
    name = "node-registry"
    ladder = ["r1", "r2", "r3"]

    def __init__(self, ctx):
        super().__init__(ctx)
        self.app = "/tmp/selfheal-apps/node-registry"
        self.sdk_repo_path = self.app  # what the SDK is told the repo is
        self.pipeline_path = self.app  # what build/ingest/search are given

    def prepare(self):
        shutil.rmtree(os.path.realpath(self.app), ignore_errors=True)
        shutil.rmtree(self.app, ignore_errors=True)
        os.makedirs(self.app, exist_ok=True)
        for fname, body in (
            ("registry.js", REGISTRY_JS),
            ("handlers.js", HANDLERS_JS),
            ("main.js", MAIN_JS),
        ):
            with open(os.path.join(self.app, fname), "w") as f:
                f.write(body)

    def profile(self, window_mult=1):
        work_ms = BASE_WORK_MS * window_mult
        rc, out = self.run(
            ["node", "main.js"],
            cwd=self.sdk_repo_path,
            env={
                "SELFHEAL_SDK": os.path.join(self.ctx["repo_root"], "sdk", "node", "index.js"),
                "SELFHEAL_REPO": self.sdk_repo_path,
                "SELFHEAL_WORK_MS": str(work_ms),
            },
            timeout=30 + (work_ms // 1000) * 4,
        )
        if rc != 0:
            raise StepError("node profile run failed (rc=%d):\n%s" % (rc, out))
        if not self.spool_files():
            raise StepError("profile run produced no spool file")

    def pipeline(self, resolved=False, rebuild=None):
        root = os.path.realpath(self.pipeline_path) if resolved else self.pipeline_path
        if rebuild:
            self.wipe_index(os.path.join(os.path.realpath(self.pipeline_path), ".codeindex"), rebuild)
        rc, out = self.run([self.ctx["bin"], "build", root], timeout=300)
        if rc != 0:
            raise StepError("codeindex build failed (rc=%d):\n%s" % (rc, out))
        rc, out = self.run([self.ctx["bin"], "ingest", root], timeout=120)
        self.ctx["log"]("    " + out.strip().replace("\n", "\n    "))
        if rc != 0:
            raise StepError("codeindex ingest failed (rc=%d):\n%s" % (rc, out))
        self.record_ingest(out)

    def verify(self):
        rc, out = self.run(
            [self.ctx["bin"], "search", self.pipeline_path, QUERY, "--limit", "5"],
            timeout=120,
        )
        self.ctx["log"]("    " + out.strip().replace("\n", "\n    "))
        blocks = cluster_blocks(out)
        return {
            "resolution_ge_30pct": (self.resolution_pct or 0) >= 30,
            "observed_edges_ge_1": (self.edges or 0) >= 1,
            "search_has_observed_marker": rc == 0 and "[observed" in out,
            "dispatcher_handler_clustered": any(
                "dispatchEvent" in b and "handleOrderShipped" in b for b in blocks
            ),
        }

    def spool_files(self):
        d = os.path.join(os.path.realpath(self.pipeline_path), ".codeindex", "runtime")
        try:
            return sorted(
                os.path.join(d, f) for f in os.listdir(d) if f.endswith(".cxprof.jsonl")
            )
        except OSError:
            return []
