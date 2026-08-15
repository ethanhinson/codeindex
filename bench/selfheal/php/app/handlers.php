<?php
/**
 * Business handlers wired into the hook registry by name.
 * Each does real CPU work so a sampling profiler catches it on-stack.
 */

/** Settle an invoice: recompute line-item totals with tax rounding. */
function handle_invoice_settled(int $invoiceId)
{
    $total = 0.0;
    for ($i = 1; $i <= 220000; $i++) {
        $line = ($invoiceId * $i) % 977;
        $total += round($line * 1.0825, 2) / ($i % 13 + 1);
        $total = fmod($total, 1000003.0);
    }
    return $total;
}

/** Register a user: derive a cheap deterministic checksum of the profile. */
function handle_user_registered(int $userId)
{
    $acc = 1;
    for ($i = 1; $i <= 220000; $i++) {
        $acc = ($acc * 31 + $userId + $i) % 2147483647;
        $acc ^= ($acc >> 7);
        $acc = abs($acc) % 2147483647 + 1;
    }
    return $acc;
}

/** Ship an order: score routes by simulated distance/cost tradeoff. */
function handle_order_shipped(int $orderId)
{
    $best = PHP_FLOAT_MAX;
    for ($i = 1; $i <= 220000; $i++) {
        $dist = sqrt(($orderId % 89) ** 2 + ($i % 97) ** 2);
        $cost = $dist * 0.42 + log($i + 1) * 3.1;
        if ($cost < $best) {
            $best = $cost;
        }
    }
    return $best;
}
