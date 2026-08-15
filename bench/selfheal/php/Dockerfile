FROM php:8.3-cli

# Excimer is a low-overhead sampling profiler for PHP (used by Wikimedia).
RUN pecl install excimer \
    && docker-php-ext-enable excimer

WORKDIR /app
