<?php
/**
 * Excimer -> cxprof v1 adapter.
 *
 * Profiles the app's event loop with ExcimerProfiler and writes a cxprof v1
 * JSONL file into the spool directory (tmp + rename, so readers never see a
 * partial file).
 *
 * Empirical Excimer facts this code relies on (verified with php -r in the
 * lab container, php 8.3 + excimer 1.2.x):
 *   - ExcimerLogEntry::getTrace() returns frames INNERMOST-FIRST
 *     (index 0 is the leaf). cxprof wants innermost LAST, so we reverse.
 *   - Each frame is ['file' => string, 'line' => int, 'function' => ?string];
 *     internal/eval frames can lack file/line, so guard for that.
 *   - getEventCount() is how many sampling events the entry represents.
 */

$appRoot  = rtrim(getenv('APP_ROOT') ?: '/app', '/');
$spoolDir = rtrim(getenv('SPOOL_DIR') ?: '/spool', '/');
$period   = 0.005; // seconds per sample => ~200 hz

require $appRoot . '/index.php';

$profiler = new ExcimerProfiler();
$profiler->setPeriod($period);
$profiler->setEventType(EXCIMER_REAL);
$profiler->setMaxDepth(100);

$start = time();
$profiler->start();
run_event_loop(2.0);
$profiler->stop();
$end = time();

$prefix = $appRoot . '/';
$stacks = []; // json-encoded frames => sample count
$frameCache = [];

foreach ($profiler->getLog() as $entry) {
    $frames = [];
    foreach ($entry->getTrace() as $frame) {
        if (!isset($frame['file'], $frame['line'])) {
            continue; // internal frame with no source location
        }
        $file = $frame['file'];
        if (strncmp($file, $prefix, strlen($prefix)) !== 0) {
            continue; // outside the app (this adapter, php internals)
        }
        // Repo-relative path: container mount prefix stripped so the host
        // index can resolve "handlers.php" etc. directly.
        $frames[] = [substr($file, strlen($prefix)), (int) $frame['line']];
    }
    if (!$frames) {
        continue;
    }
    // Excimer order is innermost-first; cxprof wants innermost LAST.
    $frames = array_reverse($frames);
    $key = json_encode($frames);
    $stacks[$key] = ($stacks[$key] ?? 0) + max(1, $entry->getEventCount());
}

$header = [
    'cxprof' => 1,
    'lang'   => 'php',
    'unit'   => 'samples',
    'hz'     => (int) round(1 / $period),
    'start'  => $start,
    'end'    => max($end, $start + 1),
    'tag'    => 'dev',
];

$out = json_encode($header) . "\n";
foreach ($stacks as $key => $n) {
    $out .= json_encode(['st' => json_decode($key), 'n' => $n]) . "\n";
}

$final = sprintf('%s/%d-php.cxprof.jsonl', $spoolDir, $start);
$tmp   = $final . '.tmp';
if (file_put_contents($tmp, $out) === false || !rename($tmp, $final)) {
    fwrite(STDERR, "adapter: failed to write $final\n");
    exit(1);
}

fwrite(STDERR, sprintf(
    "adapter: wrote %s (%d unique stacks)\n", $final, count($stacks)
));
