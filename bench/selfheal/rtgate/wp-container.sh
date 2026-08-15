#!/bin/sh
# Runs INSIDE the rtgate container.
# /repo   = bench/repos/wordpress (ro, wordpress-develop: servable tree is src/)
# /rtgate = this directory (ro)
# /spool  = spool output (rw)
#
# Builds a symlink shadow docroot at /wp so we can add wp-config.php and the
# db.php drop-in WITHOUT touching the mounted repo. PHP resolves included
# files to their realpath (/repo/src/...), so profiler frames come out
# repo-relative ("src/wp-includes/...") after stripping RTGATE_PREFIX=/repo.
set -eu

mkdir -p /wp/wp-content /wpdb

for f in /repo/src/*; do
  base="$(basename "$f")"
  [ "$base" = "wp-content" ] && continue
  ln -sfn "$f" "/wp/$base"
done
for f in /repo/src/wp-content/*; do
  ln -sfn "$f" "/wp/wp-content/$(basename "$f")"
done

cp /rtgate/wp-db-dropin.php /wp/wp-content/db.php
cp /rtgate/wp-config-rtgate.php /wp/wp-config.php

export RTGATE_PREFIX=/repo
export RTGATE_SPOOL=/spool

echo "== phase 1: install (profiled) =="
RTGATE_NAME=wordpress-install php -d auto_prepend_file=/rtgate/profiler.php /rtgate/wp-install.php

echo "== phase 2: request workload (profiled) =="
RTGATE_NAME=wordpress-request php -d auto_prepend_file=/rtgate/profiler.php /rtgate/wp-driver.php

echo "== done =="
