FROM php:7.4.3-fpm-alpine3.11@sha256:a748390f2d9f006a0bed261f751656ea49c8f040f664038fe7a47bab44f61212

RUN echo $'[www] \n\
user = www-data \n\
group = www-data \n\
listen = 127.0.0.1:9000 \n\
pm = dynamic \n\
pm.max_children = 5 \n\
pm.start_servers = 2 \n\
pm.min_spare_servers = 1 \n\
pm.max_spare_servers = 3' > /usr/local/etc/php-fpm.d/www.conf

RUN mkdir -p /app/other/dir
RUN echo "<?php echo 'Hello, World!';" > /app/index.php
RUN echo "<?php echo 'Test1 #' . time();" > /app/test1.php
RUN echo "<?php echo 'Test2 #' . time();" > /app/other/dir/test2.php
WORKDIR /app
