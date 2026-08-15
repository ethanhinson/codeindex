"""Self-healing validation scenarios for the codeindex runtime-evidence pipeline.

Each scenario is a small generator that builds a temp app, profiles it (or
delegates to an external lab), runs the pipeline (build + ingest), and
verifies assertions. The harness owns the remediation ladder; scenarios
declare which rungs are applicable via `ladder`.

Step contract (each raises StepError on hard failure, SkipScenario to skip):
  prepare()                      -- create/reset the temp app
  profile(window_mult=1)        -- produce a cxprof spool (window_mult scales work)
  pipeline(resolved=False, rebuild=None)
                                 -- build + ingest. resolved=True uses the
                                    symlink-resolved app path. rebuild:
                                    None | "ledger" (rm graph.db) |
                                    "full" (rm .codeindex, spool preserved)
  verify() -> {assertion: bool} -- deterministic assertion map
  spool_files() -> [paths]      -- what r4 quarantines
"""

import os
import re
import shutil
import subprocess


class SkipScenario(Exception):
    """Scenario cannot run in this environment; not a failure."""


class StepError(Exception):
    """A scenario step failed hard (non-zero exit, timeout, missing artifact)."""


INGEST_RE = re.compile(r"->\s+(\d+)\s+observed edges;\s+frame resolution\s+(\d+)%")


class Scenario:
    name = "base"
    optional = False
    ladder = ["r1", "r2", "r3"]  # applicable rungs, in ladder order

    def __init__(self, ctx):
        # ctx: {"repo_root", "bin", "selfheal_dir", "log": fn(str)}
        self.ctx = ctx
        self.resolution_pct = None
        self.edges = None

    # -- steps ------------------------------------------------------------
    def prepare(self):
        raise NotImplementedError

    def profile(self, window_mult=1):
        raise NotImplementedError

    def pipeline(self, resolved=False, rebuild=None):
        raise NotImplementedError

    def verify(self):
        raise NotImplementedError

    def spool_files(self):
        return []

    # -- helpers ----------------------------------------------------------
    def run(self, cmd, cwd=None, env=None, timeout=180):
        """Run a command; return (rc, combined_output). Never raises on rc!=0."""
        full_env = dict(os.environ)
        if env:
            full_env.update(env)
        self.ctx["log"]("    $ " + " ".join(cmd))
        try:
            p = subprocess.run(
                cmd, cwd=cwd, env=full_env, timeout=timeout,
                stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True,
            )
            return p.returncode, p.stdout
        except subprocess.TimeoutExpired as e:
            out = e.stdout if isinstance(e.stdout, str) else (e.stdout or b"").decode("utf-8", "replace")
            return 124, out + "\n[timeout after %ss]" % timeout

    def parse_ingest(self, out):
        """Fold ingest report lines into (edges_sum, max_resolution).

        Returns (None, None) when nothing fresh was ingested (e.g. all spool
        files were already in the ledger) so callers can keep prior values.
        """
        edges, res = None, None
        for m in INGEST_RE.finditer(out):
            edges = (edges or 0) + int(m.group(1))
            res = max(res or 0, int(m.group(2)))
        return edges, res

    def record_ingest(self, out):
        edges, res = self.parse_ingest(out)
        if edges is not None:
            self.edges, self.resolution_pct = edges, res

    def wipe_index(self, codeindex_dir, mode):
        """Reset index state, always preserving spool files.

        mode "ledger": remove graph.db (+status.json) only.
        mode "full":  stash spool, rm -rf .codeindex, restore spool.
        """
        if mode == "ledger":
            for f in ("graph.db", "status.json"):
                p = os.path.join(codeindex_dir, f)
                if os.path.exists(p):
                    os.remove(p)
            return
        if mode == "full":
            runtime = os.path.join(codeindex_dir, "runtime")
            stash = codeindex_dir + ".spool-stash"
            shutil.rmtree(stash, ignore_errors=True)
            if os.path.isdir(runtime):
                shutil.copytree(runtime, stash)
            shutil.rmtree(codeindex_dir, ignore_errors=True)
            if os.path.isdir(stash):
                os.makedirs(os.path.dirname(runtime), exist_ok=True)
                shutil.copytree(stash, runtime)
                shutil.rmtree(stash, ignore_errors=True)


def cluster_blocks(search_out):
    """Split codeindex search output into per-cluster text blocks."""
    blocks, cur = [], None
    for line in search_out.splitlines():
        if line.startswith("cluster "):
            if cur is not None:
                blocks.append("\n".join(cur))
            cur = [line]
        elif cur is not None:
            cur.append(line)
    if cur is not None:
        blocks.append("\n".join(cur))
    return blocks
