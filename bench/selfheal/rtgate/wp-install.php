<?php
/**
 * Phase 1: install WordPress into the SQLite database (profiled via
 * auto_prepend_file=profiler.php — the install itself is real core code:
 * dbDelta, schema, option/user machinery).
 */
error_reporting( E_ALL & ~E_DEPRECATED & ~E_NOTICE & ~E_WARNING );

$_SERVER['HTTP_HOST']       = 'localhost';
$_SERVER['SERVER_NAME']     = 'localhost';
$_SERVER['SERVER_PORT']     = '80';
$_SERVER['REQUEST_URI']     = '/';
$_SERVER['REQUEST_METHOD']  = 'GET';
$_SERVER['REMOTE_ADDR']     = '127.0.0.1';
$_SERVER['SERVER_PROTOCOL'] = 'HTTP/1.1';
$_SERVER['HTTP_USER_AGENT'] = 'rtgate';

define( 'WP_INSTALLING', true );

// wp-load.php is a symlink into the ro repo mount; its __DIR__ realpath would
// put ABSPATH inside /repo/src (no wp-config/db.php there). Pin it to the
// shadow docroot instead — wp-load's own define is guarded by !defined().
define( 'ABSPATH', '/wp/' );

require '/wp/wp-load.php';

if ( is_blog_installed() ) {
	fwrite( STDERR, "wp-install: already installed\n" );
	exit( 0 );
}

require_once ABSPATH . 'wp-admin/includes/upgrade.php';
$result = wp_install( 'RT Gate', 'admin', 'admin@example.com', true, '', 'password123' );
fwrite( STDERR, 'wp-install: installed, url=' . $result['url'] . "\n" );
