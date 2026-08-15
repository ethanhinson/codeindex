<?php
/**
 * Phase 2: profiled workload against an installed WordPress.
 *
 * Exercises the hook machinery (do_action/apply_filters in
 * wp-includes/plugin.php) into real core callbacks: full bootstrap
 * (plugins_loaded/init/wp_loaded), WP_Query, the_content filter chain
 * (wptexturize/wpautop/shortcodes), kses, post insertion, REST route
 * registration + dispatch, and a themed front-page render through
 * template-loader.php.
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

define( 'WP_USE_THEMES', true );

// wp-load.php is a symlink into the ro repo mount; its __DIR__ realpath would
// put ABSPATH inside /repo/src (no wp-config/db.php there). Pin it to the
// shadow docroot instead — wp-load's own define is guarded by !defined().
define( 'ABSPATH', '/wp/' );

// Full front-end bootstrap: fires plugins_loaded, init, wp_loaded, ... with
// every default core callback (create_initial_post_types, widgets, blocks).
require '/wp/wp-load.php';

$big_html = str_repeat(
	'<p>Hello <strong>world</strong> &amp; friends -- "curly quotes" ... '
	. '<a href="http://example.com/x?y=1&z=2">a link</a> <em>emphasis</em> '
	. '<img src="http://example.com/i.png" alt="x" /> [caption]captioned[/caption]</p>' . "\n\n",
	30
);

// Seed some content.
$post_ids = array();
for ( $i = 0; $i < 8; $i++ ) {
	$id = wp_insert_post(
		array(
			'post_title'   => "Profiled post $i " . md5( (string) $i ),
			'post_content' => $big_html . "<p>variant $i</p>",
			'post_status'  => 'publish',
			'post_author'  => 1,
		)
	);
	if ( ! is_wp_error( $id ) && $id ) {
		$post_ids[] = $id;
	}
}
fwrite( STDERR, 'wp-driver: seeded ' . count( $post_ids ) . " posts\n" );

// Set up the main query once, like index.php would.
wp();

$deadline = microtime( true ) + 7.0;
$iter     = 0;
$renders  = 0;
while ( microtime( true ) < $deadline ) {
	$iter++;
	try {
		// Query paths.
		$q = new WP_Query(
			array(
				'post_type'      => 'post',
				'posts_per_page' => 5,
				's'              => ( $iter % 2 ) ? 'Profiled' : 'world',
				'orderby'        => ( $iter % 3 ) ? 'date' : 'title',
			)
		);
		while ( $q->have_posts() ) {
			$q->the_post();
			ob_start();
			the_title();
			the_content();
			the_excerpt();
			ob_end_clean();
		}
		wp_reset_postdata();

		// Filter/sanitize hot paths (each crosses apply_filters in plugin.php).
		apply_filters( 'the_content', $big_html );
		wp_kses_post( $big_html );
		wptexturize( $big_html . $iter );
		wpautop( $big_html );
		sanitize_title( 'Some Title -- with "many" <em>entities</em> & symbols ' . $iter );
		esc_html( $big_html );
		do_shortcode( '[gallery ids="1,2,3"][caption]hello[/caption]' . $big_html );
		convert_smilies( $big_html . ' :) :P' );

		// Action dispatch into core callbacks (re-registers post types, etc).
		do_action( 'init' );
		do_action( 'wp_loaded' );

		if ( $post_ids ) {
			get_permalink( $post_ids[ $iter % count( $post_ids ) ] );
			get_post( $post_ids[ $iter % count( $post_ids ) ] );
		}
		if ( 0 === $iter % 4 ) {
			wp_update_post(
				array(
					'ID'           => $post_ids[ $iter % max( 1, count( $post_ids ) ) ],
					'post_content' => $big_html . "<p>rev $iter</p>",
				)
			);
		}

		// REST: registers all core routes on first call, then a real dispatch.
		if ( 1 === $iter ) {
			rest_get_server();
		}
		if ( 0 === $iter % 3 ) {
			rest_do_request( new WP_REST_Request( 'GET', '/wp/v2/posts' ) );
		}

		// Themed front page render through template-loader (block theme:
		// template resolution, block parsing/rendering, wp_head hooks).
		if ( $renders < 4 ) {
			$renders++;
			ob_start();
			require ABSPATH . WPINC . '/template-loader.php';
			ob_end_clean();
		}
	} catch ( Throwable $e ) {
		while ( ob_get_level() > 0 ) {
			ob_end_clean();
		}
		fwrite( STDERR, 'wp-driver: iter ' . $iter . ' threw: ' . $e->getMessage() . "\n" );
	}
}
fwrite( STDERR, "wp-driver: completed $iter iterations, $renders template renders\n" );
