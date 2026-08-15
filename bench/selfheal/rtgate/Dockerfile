FROM php:8.3-cli

# gd is a hard platform requirement of drupal/core's composer.json and its
# installer requirements check; excimer is the sampling profiler.
RUN apt-get update && apt-get install -y --no-install-recommends \
        git unzip libpng-dev libjpeg62-turbo-dev libwebp-dev libfreetype6-dev \
    && docker-php-ext-configure gd --with-jpeg --with-webp --with-freetype \
    && docker-php-ext-install -j"$(nproc)" gd \
    && pecl install excimer \
    && docker-php-ext-enable excimer \
    && rm -rf /var/lib/apt/lists/*

COPY --from=composer:2 /usr/bin/composer /usr/local/bin/composer
ENV COMPOSER_ALLOW_SUPERUSER=1

# SQLite drop-in database driver for WordPress (avoids MySQL entirely).
RUN git clone --depth 1 https://github.com/WordPress/sqlite-database-integration.git /sqlite

WORKDIR /work
