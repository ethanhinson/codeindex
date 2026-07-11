// codeindex VS Code extension: detect → consent → index → render → keep warm.
// All indexing lives in the codeindex binary; this file is wiring. The pure
// logic (JSONL parsing, status interpretation, debounce) lives in core.ts.

import * as vscode from "vscode";
import { execFile, spawn } from "child_process";
import {
  interpretStatus,
  parseProgressLines,
  Serializer,
  SUPPORTED_EXTENSIONS,
} from "./core";

const CONSENT_KEY = "codeindex.consent"; // "granted" | "declined"

let statusBar: vscode.StatusBarItem;
let binaryWarned = false;

export function activate(context: vscode.ExtensionContext): void {
  const config = () => vscode.workspace.getConfiguration("codeindex");
  if (!config().get<boolean>("enable", true)) return;

  statusBar = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Left, 50);
  statusBar.name = "codeindex";
  context.subscriptions.push(statusBar);

  context.subscriptions.push(
    vscode.commands.registerCommand("codeindex.indexNow", () => {
      void context.workspaceState.update(CONSENT_KEY, "granted");
      void indexWorkspace(context, true);
    }),
    vscode.commands.registerCommand("codeindex.showStatus", () => void showStatus(context)),
  );

  const root = workspaceRoot();
  if (!root) return;


  // Any save triggers the (debounced, serialized) refresh: the engine
  // content-sniffs odd extensions (PHP in .inc/.module/anything), so the
  // extension cannot predict which files matter — and a no-op refresh is
  // milliseconds. Association matching stays available for prompts/UX.
  const refresher = new Serializer(() => runRefresh(context, root), 1500);
  context.subscriptions.push(
    vscode.workspace.onDidSaveTextDocument((doc) => {
      if (!config().get<boolean>("keepFresh", true)) return;
      if (context.workspaceState.get<string>(CONSENT_KEY) !== "granted") return;
      if (doc.uri.scheme === "file") refresher.trigger();
    }),
  );

  void detectAndMaybeIndex(context);
}

export function deactivate(): void {}

function workspaceRoot(): string | undefined {
  return vscode.workspace.workspaceFolders?.[0]?.uri.fsPath;
}

async function detectAndMaybeIndex(context: vscode.ExtensionContext): Promise<void> {
  const root = workspaceRoot();
  if (!root) return;
  const bin = await findBinary();
  if (!bin) return;

  const state = interpretStatus(await statusJSON(bin, root));
  switch (state.kind) {
    case "indexed":
      setBarFresh(state.files, state.symbols);
      return;
    case "building":
      // Another process (CI import? a CLI build?) owns it; just reflect.
      setBarBusy(state.done, state.total);
      return;
    case "unindexed":
    case "needs-reindex":
      break;
  }

  const auto = vscode.workspace.getConfiguration("codeindex").get<string>("autoIndex", "prompt");
  if (auto === "never") return;
  const consent = context.workspaceState.get<string>(CONSENT_KEY);
  if (consent === "declined") return;

  if (auto === "prompt" && consent !== "granted") {
    const files = await vscode.workspace.findFiles(
      `**/*.{${SUPPORTED_EXTENSIONS.map((e) => e.slice(1)).join(",")}}`,
      "**/node_modules/**",
      20000,
    );
    const approx = files.length >= 20000 ? "20,000+" : String(files.length);
    const reason = state.kind === "needs-reindex" ? ` (${state.reason})` : "";
    const pick = await vscode.window.showInformationMessage(
      `codeindex: this workspace isn't indexed${reason} — ${approx} source files. Index now so AI agents can query the symbol graph instantly?`,
      "Index",
      "Not this workspace",
    );
    if (pick !== "Index") {
      if (pick === "Not this workspace") {
        void context.workspaceState.update(CONSENT_KEY, "declined");
      }
      return;
    }
    void context.workspaceState.update(CONSENT_KEY, "granted");
  }
  await indexWorkspace(context, false);
}

// indexWorkspace runs `codeindex build --progress`, rendering the JSONL feed
// into a progress notification + the status bar.
async function indexWorkspace(context: vscode.ExtensionContext, manual: boolean): Promise<void> {
  const root = workspaceRoot();
  if (!root) return;
  const bin = await findBinary();
  if (!bin) return;

  await vscode.window.withProgress(
    {
      location: vscode.ProgressLocation.Notification,
      title: "codeindex: indexing workspace",
      cancellable: true,
    },
    (progress, token) =>
      new Promise<void>((resolve) => {
        const child = spawn(bin, ["build", root, "--progress"], { cwd: root });
        token.onCancellationRequested(() => child.kill());

        let buf = "";
        let lastPct = 0;
        child.stdout.on("data", (chunk: Buffer) => {
          buf += chunk.toString();
          const { events, rest } = parseProgressLines(buf);
          buf = rest;
          for (const e of events) {
            if (e.phase === "done") continue;
            if (e.total && e.total > 0 && e.done !== undefined) {
              const pct = Math.floor((e.done / e.total) * 100);
              progress.report({
                message: `${e.phase} ${e.done}/${e.total}`,
                increment: pct - lastPct,
              });
              lastPct = pct;
              setBarBusy(e.done, e.total, e.phase);
            } else {
              progress.report({ message: e.phase });
              setBarBusy(undefined, undefined, e.phase);
            }
          }
        });
        child.on("close", (code) => {
          if (code === 0) {
            void refreshBarFromStatus(bin, root);
            if (manual) void vscode.window.showInformationMessage("codeindex: workspace indexed.");
          } else if (!token.isCancellationRequested) {
            statusBar.text = "$(warning) codeindex";
            statusBar.tooltip = `indexing failed (exit ${code}) — see codeindex build output`;
            statusBar.show();
            void vscode.window.showWarningMessage(`codeindex: indexing failed (exit ${code}).`);
          }
          resolve();
        });
        child.on("error", () => resolve());
      }),
  );
}

async function runRefresh(context: vscode.ExtensionContext, root: string): Promise<void> {
  const bin = await findBinary();
  if (!bin) return;
  setBarBusy(undefined, undefined, "refreshing");
  await new Promise<void>((resolve) => {
    execFile(bin, ["refresh", root], { cwd: root }, () => resolve());
  });
  await refreshBarFromStatus(bin, root);
}

async function showStatus(context: vscode.ExtensionContext): Promise<void> {
  const root = workspaceRoot();
  const bin = await findBinary();
  if (!root || !bin) return;
  const s = interpretStatus(await statusJSON(bin, root));
  const msg =
    s.kind === "indexed"
      ? `indexed: ${s.files ?? "?"} files, ${s.symbols ?? "?"} symbols`
      : s.kind === "building"
        ? `indexing: ${s.phase ?? ""} ${s.done ?? ""}/${s.total ?? ""}`
        : s.kind === "needs-reindex"
          ? `needs re-index: ${s.reason}`
          : "not indexed";
  const pick = await vscode.window.showInformationMessage(`codeindex — ${msg}`, "Index now");
  if (pick === "Index now") void vscode.commands.executeCommand("codeindex.indexNow");
}

function statusJSON(bin: string, root: string): Promise<unknown> {
  return new Promise((resolve) => {
    execFile(bin, ["status", root, "--json"], { cwd: root }, (err, stdout) => {
      if (err) return resolve(null);
      try {
        resolve(JSON.parse(stdout));
      } catch {
        resolve(null);
      }
    });
  });
}

async function refreshBarFromStatus(bin: string, root: string): Promise<void> {
  const s = interpretStatus(await statusJSON(bin, root));
  if (s.kind === "indexed") setBarFresh(s.files, s.symbols);
}

function setBarBusy(done?: number, total?: number, phase?: string): void {
  const counts = done !== undefined && total ? ` ${done}/${total}` : "";
  statusBar.text = `$(sync~spin) codeindex${counts}`;
  statusBar.tooltip = `codeindex: ${phase ?? "indexing"}${counts}`;
  statusBar.command = "codeindex.showStatus";
  statusBar.show();
}

function setBarFresh(files?: number, symbols?: number): void {
  statusBar.text = "$(check) codeindex";
  statusBar.tooltip = `codeindex: fresh — ${files ?? "?"} files, ${symbols ?? "?"} symbols. Click for status.`;
  statusBar.command = "codeindex.showStatus";
  statusBar.show();
}

// findBinary honors the setting, else probes PATH. Warns once per session
// with actionable buttons when missing.
async function findBinary(): Promise<string | undefined> {
  const configured = vscode.workspace
    .getConfiguration("codeindex")
    .get<string>("binaryPath", "")
    .trim();
  const candidate = configured || "codeindex";
  const ok = await new Promise<boolean>((resolve) => {
    execFile(candidate, ["--help"], (err) => resolve(!isMissing(err)));
  });
  if (ok) return candidate;

  if (!binaryWarned) {
    binaryWarned = true;
    void vscode.window
      .showWarningMessage(
        "codeindex binary not found. Install it or point codeindex.binaryPath at it.",
        "How to install",
        "Open settings",
      )
      .then((pick) => {
        if (pick === "How to install") {
          void vscode.env.openExternal(vscode.Uri.parse("https://github.com/ethanhinson/codeindex#build"));
        } else if (pick === "Open settings") {
          void vscode.commands.executeCommand("workbench.action.openSettings", "codeindex.binaryPath");
        }
      });
  }
  return undefined;
}

function isMissing(err: unknown): boolean {
  return !!err && (err as NodeJS.ErrnoException).code === "ENOENT";
}
