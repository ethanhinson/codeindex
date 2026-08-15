<?php
/**
 * rtgate Excimer -> cxprof v1 profiler shim.
 *
 * Designed to run as an auto_prepend_file so any PHP entrypoint (our drivers,
 * Drupal's console script, ...) is profiled end to end, including scripts that
 * exit()/die(): the profile is flushed from a shutdown function.
 *
 * Env contract:
 *   RTGATE_PREFIX  container path prefix of the mounted repo (default /repo).
 *                  Frames under it are emitted repo-relative; all other frames
 *                  are dropped (adapter, vendor, sqlite shim, php internals).
 *   RTGATE_SPOOL   spool dir (default /spool)
 *   RTGATE_NAME    spool file basename component (default php)
 *   RTGATE_COMMIT  optional VCS commit for the header
 *   RTGATE_PERIOD  sampling period in seconds (default 0.003 ~ 333hz)
 *
 * Empirical Excimer facts (proven in bench/selfheal/php, php8.3+excimer):
 *   - ExcimerLogEntry::getTrace() is innermost-FIRST; cxprof wants innermost
 *     LAST, so each stack is reversed.
 *   - Frames are ['file','line','function']; internal frames may lack
 *     file/line and are skipped.
 *   - getEventCount() may coalesce >1 sampling event per entry.
 */

if (!extension_loaded('excimer')) {
    fwrite(STDERR, "rtgate profiler: excimer not loaded, not profiling\n");
    return;
}

$GLOBALS['__rtgate_period'] = (float) (getenv('RTGATE_PERIOD') ?: 0.003);
$GLOBALS['__rtgate_start']  = time();

$__rtgate_prof = new ExcimerProfiler();
$__rtgate_prof->setPeriod($GLOBALS['__rtgate_period']);
$__rtgate_prof->setEventType(EXCIMER_REAL);
$__rtgate_prof->setMaxDepth(150);
$GLOBALS['__rtgate_prof'] = $__rtgate_prof;
$__rtgate_prof->start();
// NB: do NOT unset($__rtgate_prof) here — auto_prepend runs at global scope,
// so the local IS the global and unsetting it would kill $GLOBALS too.

register_shutdown_function(function (): void {
    /** @var ExcimerProfiler $prof */
    $prof = $GLOBALS['__rtgate_prof'];
    $prof->stop();
    $end = time();

    $prefix = rtrim(getenv('RTGATE_PREFIX') ?: '/repo', '/') . '/';
    $spool  = rtrim(getenv('RTGATE_SPOOL') ?: '/spool', '/');
    $name   = getenv('RTGATE_NAME') ?: 'php';
    $commit = getenv('RTGATE_COMMIT') ?: '';
    $plen   = strlen($prefix);

    $stacks = []; // json-encoded frame list => summed sample count
    $total  = 0;
    foreach ($prof->getLog() as $entry) {
        $frames = [];
        foreach ($entry->getTrace() as $frame) {
            if (!isset($frame['file'], $frame['line'])) {
                continue; // internal frame without a source location
            }
            $file = $frame['file'];
            if (strncmp($file, $prefix, $plen) !== 0) {
                continue; // outside the repo mount: shim, vendor, drop-ins
            }
            $frames[] = [substr($file, $plen), (int) $frame['line']];
        }
        if (!$frames) {
            continue;
        }
        $frames = array_reverse($frames); // excimer innermost-first -> cxprof innermost-last
        $key = json_encode($frames);
        $n = max(1, $entry->getEventCount());
        $stacks[$key] = ($stacks[$key] ?? 0) + $n;
        $total += $n;
    }

    $header = [
        'cxprof' => 1,
        'lang'   => 'php',
        'unit'   => 'samples',
        'hz'     => (int) round(1 / $GLOBALS['__rtgate_period']),
        'start'  => $GLOBALS['__rtgate_start'],
        'end'    => max($end, $GLOBALS['__rtgate_start'] + 1),
        'tag'    => 'gate',
    ];
    if ($commit !== '') {
        $header['commit'] = $commit;
    }

    $out = json_encode($header) . "\n";
    foreach ($stacks as $key => $n) {
        $out .= json_encode(['st' => json_decode($key), 'n' => $n]) . "\n";
    }

    $final = sprintf('%s/%s-%d-%d.cxprof.jsonl', $spool, $name, $GLOBALS['__rtgate_start'], getmypid());
    $tmp   = $final . '.tmp';
    if (file_put_contents($tmp, $out) === false || !rename($tmp, $final)) {
        fwrite(STDERR, "rtgate profiler: FAILED to write $final\n");
        return;
    }
    fwrite(STDERR, sprintf(
        "rtgate profiler: wrote %s (%d unique stacks, %d samples)\n",
        $final, count($stacks), $total
    ));
});
