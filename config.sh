#! /bin/bash

host=$1
if [ -z "$host" ]; then
    host=http://127.0.0.1:2379
fi

root=/v2/keys/skipper

# backends:
curl -k -X PUT -d 'value="https://header.mop-taskforce.zalan.do"' "$host""$root"/backends/header
curl -k -X PUT -d 'value="https://footer.mop-taskforce.zalan.do"' "$host""$root"/backends/footer
curl -k -X PUT -d 'value="https://cart.mop-taskforce.zalan.do"' "$host""$root"/backends/cart
curl -k -X PUT -d 'value="https://layout-service-9.mop-taskforce.zalan.do"' "$host""$root"/backends/layout-service-9
curl -k -X PUT -d 'value="https://bugfactory.mop-taskforce.zalan.do"' "$host""$root"/backends/debug

# fronteds:
curl -k -X PUT -d 'value={"route": "Path(`/tessera/header`)", "backend-id": "header", "filters": ["xalando", "cut-path", "header-host"]}' "$host""$root"/frontends/header
curl -k -X PUT -d 'value={"route": "Path(`/tessera/footer`)", "backend-id": "footer", "filters": ["xalando", "cut-path", "footer-host"]}' "$host""$root"/frontends/footer
curl -k -X PUT -d 'value={"route": "PathRegexp(`.*\\.html`)", "backend-id": "layout-service-9", "filters": ["xalando", "layout-service-host", "pdp-path"]}' "$host""$root"/frontends/pdp
curl -k -X PUT -d 'value={"route": "Path(`/<string>`)", "backend-id": "layout-service-9", "filters": ["xalando", "layout-service-host", "catalog-path"]}' "$host""$root"/frontends/catalog
curl -k -X PUT -d 'value={"route": "Path(`/slow`)", "backend-id": "debug", "filters": ["xalando", "bugfactory-host"]}' "$host""$root"/frontends/slow
curl -k -X PUT -d 'value={"route": "Path(`/debug`)", "backend-id": "debug", "filters": ["xalando", "cut-path", "bugfactory-host"]}' "$host""$root"/frontends/debug
curl -k -X PUT -d 'value={"route": "PathRegexp(`/api/cart/.*`)", "backend-id": "cart", "filters": ["xalando", "cart-host"]}' "$host""$root"/frontends/cart

# filter specs:
CURL -k -X PUT -d 'value={"middleware-name": "xalando"}' "$host""$root"/filter-specs/xalando
curl -k -X PUT -d 'value={"middleware-name": "request-header", "config": {"key": "Host", "value": "header.mop-taskforce.zalan.do"}}' "$host""$root"/filter-specs/header-host
curl -k -X PUT -d 'value={"middleware-name": "request-header", "config": {"key": "Host", "value": "footer.mop-taskforce.zalan.do"}}' "$host""$root"/filter-specs/footer-host
curl -k -X PUT -d 'value={"middleware-name": "request-header", "config": {"key": "Host", "value": "layout-service-9.mop-taskforce.zalan.do"}}' "$host""$root"/filter-specs/layout-service-host
curl -k -X PUT -d 'value={"middleware-name": "request-header", "config": {"key": "Host", "value": "bugfactory.mop-taskforce.zalan.do"}}' "$host""$root"/filter-specs/bugfactory-host
curl -k -X PUT -d 'value={"middleware-name": "request-header", "config": {"key": "Host", "value": "cart-taskforce.zalan.do"}}' "$host""$root"/filter-specs/cart-host
curl -k -X PUT -d 'value={"middleware-name": "path-rewrite", "config": {"expression": ".*", "replacement": "/"}}' "$host""$root"/filter-specs/cut-path
curl -k -X PUT -d 'value={"middleware-name": "path-rewrite", "config": {"expression": ".*", "replacement": "/pdp"}}' "$host""$root"/filter-specs/pdp-path
curl -k -X PUT -d 'value={"middleware-name": "path-rewrite", "config": {"expression": ".*", "replacement": "/catalog"}}' "$host""$root"/filter-specs/catalog-path
