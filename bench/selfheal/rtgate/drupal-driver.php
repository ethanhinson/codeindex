<?php
/**
 * Phase 2: profiled workload against an installed Drupal (standard profile).
 *
 * Full DrupalKernel bootstrap, then repeated front-controller requests
 * (routing, access, controller, render pipeline, Twig), entity CRUD (node
 * presave/insert hook invocations through ModuleHandler), and a cron run
 * (ModuleHandler::invokeAll('cron') — classic hook dispatch).
 */

use Drupal\Core\DrupalKernel;
use Symfony\Component\HttpFoundation\Request;

error_reporting(E_ALL & ~E_DEPRECATED);
chdir('/site');
$autoloader = require '/site/autoload.php';

$request = Request::create('http://localhost/');
$kernel = DrupalKernel::createFromRequest($request, $autoloader, 'prod');
$response = $kernel->handle($request);
$kernel->terminate($request, $response);
fwrite(STDERR, 'drupal-driver: first request status ' . $response->getStatusCode() . "\n");

// One cron run: ModuleHandler::invokeAll('cron') across installed modules.
try {
    \Drupal::service('cron')->run();
} catch (Throwable $e) {
    fwrite(STDERR, 'drupal-driver: cron threw: ' . $e->getMessage() . "\n");
}

$paths = [
    '/',
    '/user/login',
    '/user/password',
    '/user/register',
    '/search/node?keys=drupal',
    '/node',
    '/filter/tips',
    '/contact',
    '/rss.xml',
    '/no/such/page',
];

$deadline = microtime(true) + 8.0;
$iter = 0;
$codes = [];
$nids = [];
while (microtime(true) < $deadline) {
    $path = $paths[$iter % count($paths)];
    $sep = str_contains($path, '?') ? '&' : '?';
    $iter++;
    try {
        // Cache-busting param so page_cache doesn't short-circuit everything.
        $req = Request::create('http://localhost' . $path . $sep . 'v=' . $iter);
        $res = $kernel->handle($req);
        $kernel->terminate($req, $res);
        $codes[$res->getStatusCode()] = ($codes[$res->getStatusCode()] ?? 0) + 1;
    } catch (Throwable $e) {
        fwrite(STDERR, "drupal-driver: request $path threw: " . $e->getMessage() . "\n");
    }

    // Entity CRUD every few iterations: fires hook_node_presave/insert etc.
    if (0 === $iter % 3) {
        try {
            $node = \Drupal::entityTypeManager()->getStorage('node')->create([
                'type'  => ($iter % 2) ? 'article' : 'page',
                'title' => 'Profiled node ' . $iter,
                'body'  => [
                    'value'  => str_repeat('<p>Hello <strong>world</strong> &amp; entities.</p>', 20),
                    'format' => 'basic_html',
                ],
            ]);
            $node->save();
            $nids[] = $node->id();
            // Render it through the entity render pipeline.
            $view = \Drupal::entityTypeManager()->getViewBuilder('node')->view($node, 'full');
            \Drupal::service('renderer')->renderInIsolation($view);
        } catch (Throwable $e) {
            fwrite(STDERR, 'drupal-driver: entity op threw: ' . $e->getMessage() . "\n");
        }
    }
}
fwrite(STDERR, 'drupal-driver: ' . $iter . ' requests, ' . count($nids) . ' nodes, codes=' . json_encode($codes) . "\n");
