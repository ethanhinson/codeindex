<?php
/**
 * Entry point: wires business handlers to string-named events, then
 * dispatches a stream of events for ~2 seconds of wall time.
 */

require __DIR__ . '/hooks.php';
require __DIR__ . '/handlers.php';

add_action('invoice.settled', 'handle_invoice_settled');
add_action('user.registered', 'handle_user_registered');
add_action('order.shipped', 'handle_order_shipped');

function run_event_loop(float $seconds = 2.0): int
{
    $deadline = microtime(true) + $seconds;
    $dispatched = 0;
    $id = 0;
    while (microtime(true) < $deadline) {
        $id++;
        do_action('invoice.settled', $id);
        do_action('user.registered', $id);
        do_action('order.shipped', $id);
        $dispatched += 3;
    }
    return $dispatched;
}

if (realpath($_SERVER['SCRIPT_FILENAME'] ?? '') === __FILE__) {
    $n = run_event_loop(2.0);
    fwrite(STDERR, "dispatched $n events\n");
}
