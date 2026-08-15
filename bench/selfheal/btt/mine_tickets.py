#!/usr/bin/env python3
"""Mine btt-api git history: ticket refs -> fix commits -> symbol mapping
(hunk-header xfuncname vs the current index). Emits the ranked list of
tickets worth fetching. No network."""
import json, re, sqlite3, subprocess
from collections import defaultdict
from pathlib import Path

REPO = Path("bench/repos/btt-api")
DB = REPO / ".codeindex/graph.db"
TICKET = re.compile(r"\b([A-Z]{2,10}-[0-9]{1,6})\b")
FUNC = re.compile(r"(?:function\s+&?\s*([A-Za-z_]\w*)|class\s+([A-Za-z_]\w*))")

def sh(*args):
    return subprocess.run(["git", "-C", str(REPO), *args],
                          capture_output=True).stdout.decode("utf-8", errors="replace")

con = sqlite3.connect(str(DB))
def symbols_in(file):
    return {(n, p) for n, p in con.execute(
        "SELECT name, parent FROM symbols WHERE tier=0 AND file=?", (file,))}

log = sh("log", "--no-merges", "--pretty=%H\t%s", "-n", "12000")
by_ticket = defaultdict(list)
for ln in log.splitlines():
    sha, _, subj = ln.partition("\t")
    for t in TICKET.findall(subj):
        if t.split("-")[0] in ("AD", "PP", "GDE", "RES", "DSMA"):
            by_ticket[t].append((sha, subj))

out = {}
for ticket, commits in by_ticket.items():
    accepts, files, shas = set(), set(), []
    for sha, subj in commits[:3]:  # newest 3 commits per ticket
        show = sh("show", "-p", "--unified=0", "--pretty=format:", sha)
        cur = None
        touched = set()
        for ln in show.splitlines():
            if ln.startswith("+++ b/"):
                cur = ln[6:]
                if cur.endswith((".php", ".module", ".inc", ".install")):
                    touched.add(cur)
                else:
                    cur = None
            elif ln.startswith("@@") and cur:
                ctx = ln.split("@@")[-1]
                m = FUNC.search(ctx)
                if m:
                    name = m.group(1) or m.group(2)
                    for n, p in symbols_in(cur):
                        if n == name:
                            accepts.add(f"{p}.{n}" if p else n)
                            files.add(cur)
        shas.append(sha[:10])
    if accepts and 1 <= len(files) <= 5:
        out[ticket] = {"commits": shas, "n_symbols": len(accepts),
                       "accept": sorted(accepts)[:8], "files": sorted(files)[:5]}

ranked = dict(sorted(out.items(), key=lambda kv: -kv[1]["n_symbols"]))
Path("bench/selfheal/btt/tickets_needed.json").write_text(json.dumps(ranked, indent=1))
print(f"tickets scanned: {len(by_ticket)}, mappable: {len(ranked)}")
for prefix in ("AD","PP","GDE","RES","DSMA"):
    n = sum(1 for t in ranked if t.startswith(prefix+"-"))
    print(f"  {prefix}: {n}")
print("top 20 to fetch:", ", ".join(list(ranked)[:20]))
