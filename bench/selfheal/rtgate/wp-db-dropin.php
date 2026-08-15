<?php
/**
 * wp-content/db.php drop-in for the rtgate container: routes wpdb to the
 * SQLite implementation cloned at /sqlite (image build time). Lives OUTSIDE
 * the repo mount so its frames are dropped by the profiler prefix filter.
 */
define( 'SQLITE_DB_DROPIN_VERSION', '1.8.0' );
if ( ! defined( 'DATABASE_TYPE' ) ) {
	define( 'DATABASE_TYPE', 'sqlite' );
}
if ( ! defined( 'DB_ENGINE' ) ) {
	define( 'DB_ENGINE', 'sqlite' );
}
require_once '/sqlite/packages/plugin-sqlite-database-integration/wp-includes/sqlite/db.php';
