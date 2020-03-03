FROM php:7.4.3-fpm-alpine3.11

RUN echo $'[www] \n\
user = www-data \n\
group = www-data \n\
listen = 127.0.0.1:9000 \n\
pm = dynamic \n\
pm.max_children = 5 \n\
pm.start_servers = 2 \n\
pm.min_spare_servers = 1 \n\
pm.max_spare_servers = 3' > /usr/local/etc/php-fpm.d/www.conf

RUN mkdir -p /app
RUN echo "<?php echo 'Hello, World!';" > /app/index.php
WORKDIR /app