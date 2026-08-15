<?php
/**
 * Minimal WordPress-style hook registry.
 *
 * Handlers are attached to string event names via add_action() and fired
 * with do_action(). The string-keyed indirection is deliberately opaque to
 * static analysis: nothing in the source ties "invoice.settled" the string
 * to handle_invoice_settled the function except data flowing at runtime.
 */

$GLOBALS['__hooks'] = [];

function add_action(string $event, callable $handler, int $priority = 10): void
{
    $GLOBALS['__hooks'][$event][$priority][] = $handler;
}

function do_action(string $event, ...$args)
{
    if (empty($GLOBALS['__hooks'][$event])) {
        return null;
    }
    $byPriority = $GLOBALS['__hooks'][$event];
    ksort($byPriority);
    $result = null;
    foreach ($byPriority as $handlers) {
        foreach ($handlers as $handler) {
            $result = $handler(...$args);
        }
    }
    return $result;
}
