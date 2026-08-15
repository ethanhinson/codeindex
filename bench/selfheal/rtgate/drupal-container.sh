#!/bin/sh
# Runs INSIDE the rtgate container.
# /site/core = bench/repos/drupal (ro BIND MOUNT — not a symlink, on purpose:
#              DrupalKernel::guessApplicationRoot() and InstallCommand's
#              chdir(dirname(__DIR__, 5)) are realpath-based, so core must
#              physically sit at <root>/core)
# /rtgate    = this directory (ro)
# /spool     = spool output (rw)
# /deps      = docker named volume caching the composer vendor dir
#
# drupal/core lacks vendor/; we composer-install its deps in a symlink shadow
# (/deps/build-core) so nothing is written into the repo mount. The site root
# /site lives in container fs and is scaffolded from core/assets/scaffold.
set -eu

DEPS=/deps/build-core
if [ ! -f "$DEPS/vendor/autoload.php" ]; then
  echo "== composer install (cached in volume afterwards) =="
  mkdir -p "$DEPS"
  cp /site/core/composer.json "$DEPS/composer.json"
  for d in lib includes modules profiles themes recipes assets misc scripts tests; do
    [ -e "/site/core/$d" ] && ln -sfn "/site/core/$d" "$DEPS/$d"
  done
  (cd "$DEPS" && composer update --no-dev --no-scripts --no-plugins --no-interaction --no-progress 2>&1 | tail -3)
fi

mkdir -p /site/sites/default/files
cp /site/core/assets/scaffold/files/default.settings.php /site/sites/default/default.settings.php
cp /site/core/assets/scaffold/files/index.php /site/index.php
printf '<?php return require "%s/vendor/autoload.php";\n' "$DEPS" > /site/autoload.php
chmod -R 0777 /site/sites

export RTGATE_PREFIX=/site/core
export RTGATE_SPOOL=/spool

cd /site
echo "== phase 1: site install, standard profile (profiled) =="
RTGATE_NAME=drupal-install RTGATE_PERIOD=0.005 \
  php -d auto_prepend_file=/rtgate/profiler.php -d memory_limit=1G \
  core/scripts/drupal install standard --no-interaction --site-name "RT Gate"

echo "== phase 2: request workload (profiled) =="
RTGATE_NAME=drupal-request \
  php -d auto_prepend_file=/rtgate/profiler.php -d memory_limit=1G \
  /rtgate/drupal-driver.php

echo "== done =="
