<?php
/**
 * rtgate wp-config: SQLite-backed, no network, CLI-driven.
 * Placed at the docroot (/wp) which is a symlink shadow of /repo/src, so
 * ABSPATH is /wp/ but every core file resolves (realpath) to /repo/src/...
 */
define( 'DB_NAME', 'wordpress' );
define( 'DB_USER', 'wp' );
define( 'DB_PASSWORD', 'wp' );
define( 'DB_HOST', 'localhost' );
define( 'DB_CHARSET', 'utf8mb4' );
define( 'DB_COLLATE', '' );

// SQLite drop-in: database file outside the repo mount.
define( 'DB_DIR', '/wpdb/' );
define( 'DB_FILE', 'wp.sqlite' );

$table_prefix = 'wp_';

define( 'WP_DEBUG', false );
define( 'WP_HOME', 'http://localhost' );
define( 'WP_SITEURL', 'http://localhost' );
define( 'DISABLE_WP_CRON', true );
define( 'WP_HTTP_BLOCK_EXTERNAL', true );
define( 'AUTOMATIC_UPDATER_DISABLED', true );
define( 'WP_AUTO_UPDATE_CORE', false );

define( 'AUTH_KEY',         'rtgate-auth-key-0000000000000000' );
define( 'SECURE_AUTH_KEY',  'rtgate-secure-auth-key-000000000' );
define( 'LOGGED_IN_KEY',    'rtgate-logged-in-key-00000000000' );
define( 'NONCE_KEY',        'rtgate-nonce-key-000000000000000' );
define( 'AUTH_SALT',        'rtgate-auth-salt-000000000000000' );
define( 'SECURE_AUTH_SALT', 'rtgate-secure-auth-salt-00000000' );
define( 'LOGGED_IN_SALT',   'rtgate-logged-in-salt-0000000000' );
define( 'NONCE_SALT',       'rtgate-nonce-salt-00000000000000' );

if ( ! defined( 'ABSPATH' ) ) {
	define( 'ABSPATH', __DIR__ . '/' );
}
require_once ABSPATH . 'wp-settings.php';
