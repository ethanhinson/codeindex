// Pure, IDE-free logic: JSONL progress parsing, status interpretation,
// debounced serialization. Everything here is unit-tested with node; the
// extension entrypoint is just wiring.

export interface ProgressEvent {
  v: number;
  phase: string;
  done?: number;
  total?: number;
  summary?: string;
}

// parseProgressLines consumes a chunk of JSONL output (possibly ending
// mid-line) and returns complete events plus the unconsumed remainder.
export function parseProgressLines(buffer: string): { events: ProgressEvent[]; rest: string } {
  const events: ProgressEvent[] = [];
  const lines = buffer.split("\n");
  const rest = lines.pop() ?? "";
  for (const line of lines) {
    if (!line.trim()) continue;
    try {
      const e = JSON.parse(line);
      if (e && e.v === 1 && typeof e.phase === "string") events.push(e);
    } catch {
      // non-event noise on the stream is ignored, never fatal
    }
  }
  return { events, rest };
}

export type IndexState =
  | { kind: "unindexed" }
  | { kind: "needs-reindex"; reason: string }
  | { kind: "building"; phase?: string; done?: number; total?: number }
  | { kind: "indexed"; files?: number; symbols?: number };

// interpretStatus maps `codeindex status --json` output to what the
// extension should do. Unknown shapes degrade to unindexed (safe: worst
// case is one redundant consent prompt).
export function interpretStatus(raw: unknown): IndexState {
  const s = raw as Record<string, unknown> | null;
  if (!s || typeof s !== "object") return { kind: "unindexed" };
  switch (s.state) {
    case "indexed":
      return { kind: "indexed", files: num(s.files), symbols: num(s.symbols) };
    case "stale-schema":
      return { kind: "needs-reindex", reason: `index schema v${s.schema_version} != v${s.schema_required}` };
    case "building":
    case "patching":
      if (s.stale === true) return { kind: "needs-reindex", reason: "previous indexer died mid-build" };
      return { kind: "building", phase: str(s.phase), done: num(s.done), total: num(s.total) };
    default:
      return { kind: "unindexed" };
  }
}

function num(v: unknown): number | undefined {
  return typeof v === "number" ? v : undefined;
}
function str(v: unknown): string | undefined {
  return typeof v === "string" ? v : undefined;
}

// Serializer runs at most one task at a time with trailing-edge debounce:
// bursts of trigger() collapse into one run; triggers during a run queue
// exactly one follow-up (the index is a single-writer database).
export class Serializer {
  private timer: ReturnType<typeof setTimeout> | undefined;
  private running = false;
  private pending = false;

  constructor(
    private readonly task: () => Promise<void>,
    private readonly debounceMs: number,
  ) {}

  trigger(): void {
    if (this.timer !== undefined) clearTimeout(this.timer);
    this.timer = setTimeout(() => {
      this.timer = undefined;
      void this.run();
    }, this.debounceMs);
  }

  private async run(): Promise<void> {
    if (this.running) {
      this.pending = true;
      return;
    }
    this.running = true;
    try {
      await this.task();
    } finally {
      this.running = false;
      if (this.pending) {
        this.pending = false;
        void this.run();
      }
    }
  }
}

export const SUPPORTED_EXTENSIONS = [".go", ".ts", ".tsx", ".js", ".jsx", ".py", ".php"];

export function isSupportedFile(path: string): boolean {
  const lower = path.toLowerCase();
  return SUPPORTED_EXTENSIONS.some((e) => lower.endsWith(e));
}
