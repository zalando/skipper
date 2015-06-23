#! /bin/bash

curl -X PUT -d 'value="https://header.mop-taskforce.zalan.do"' http://127.0.0.1:2379/v2/keys/skipper/backends/header
curl -X PUT -d 'value="https://footer.mop-taskforce.zalan.do"' http://127.0.0.1:2379/v2/keys/skipper/backends/footer
curl -X PUT -d 'value="https://cart.mop-taskforce.zalan.do"' http://127.0.0.1:2379/v2/keys/skipper/backends/cart
curl -X PUT -d 'value="https://layout-service-9.mop-taskforce.zalan.do"' http://127.0.0.1:2379/v2/keys/skipper/backends/layout-service-9
curl -X PUT -d 'value="https://bugfactory.mop-taskforce.zalan.do"' http://127.0.0.1:2379/v2/keys/skipper/backends/debug

curl -X PUT -d 'value={"route": "Path(`/tessera/header`)", "backend-id": "header", "filters": ["xalando", "cut-path", "header-host"]}' http://127.0.0.1:2379/v2/keys/skipper/frontends/header
curl -X PUT -d 'value={"route": "Path(`/tessera/footer`)", "backend-id": "footer", "filters": ["xalando", "cut-path", "footer-host"]}' http://127.0.0.1:2379/v2/keys/skipper/frontends/footer
curl -X PUT -d 'value={"route": "PathRegexp(`.*\\.html`)", "backend-id": "layout-service-9", "filters": ["xalando", "layout-service-host", "pdp-path"]}' http://127.0.0.1:2379/v2/keys/skipper/frontends/pdp
curl -X PUT -d 'value={"route": "Path(`/<string>`)", "backend-id": "layout-service-9", "filters": ["xalando", "layout-service-host", "catalog-path"]}' http://127.0.0.1:2379/v2/keys/skipper/frontends/catalog
curl -X PUT -d 'value={"route": "Path(`/slow`)", "backend-id": "debug", "filters": ["xalando", "bugfactory-host"]}' http://127.0.0.1:2379/v2/keys/skipper/frontends/slow
curl -X PUT -d 'value={"route": "Path(`/debug`)", "backend-id": "debug", "filters": ["xalando", "cut-path", "bugfactory-host"]}' http://127.0.0.1:2379/v2/keys/skipper/frontends/debug
curl -X PUT -d 'value={"route": "PathRegexp(`/api/cart/.*`)", "backend-id": "cart", "filters": ["xalando", "cart-host"]}' http://127.0.0.1:2379/v2/keys/skipper/frontends/cart

CURL -X PUT -d 'value={"middleware-name": "xalando"}' http://127.0.0.1:2379/v2/keys/skipper/filter-specs/xalando
curl -X PUT -d 'value={"middleware-name": "request-header", "config": {"key": "Host", "value": "header.mop-taskforce.zalan.do"}}' http://127.0.0.1:2379/v2/keys/skipper/filter-specs/header-host
curl -X PUT -d 'value={"middleware-name": "request-header", "config": {"key": "Host", "value": "footer.mop-taskforce.zalan.do"}}' http://127.0.0.1:2379/v2/keys/skipper/filter-specs/footer-host
curl -X PUT -d 'value={"middleware-name": "request-header", "config": {"key": "Host", "value": "layout-service-9.mop-taskforce.zalan.do"}}' http://127.0.0.1:2379/v2/keys/skipper/filter-specs/layout-service-host
curl -X PUT -d 'value={"middleware-name": "request-header", "config": {"key": "Host", "value": "bugfactory.mop-taskforce.zalan.do"}}' http://127.0.0.1:2379/v2/keys/skipper/filter-specs/bugfactory-host
curl -X PUT -d 'value={"middleware-name": "request-header", "config": {"key": "Host", "value": "cart-taskforce.zalan.do"}}' http://127.0.0.1:2379/v2/keys/skipper/filter-specs/cart-host
curl -X PUT -d 'value={"middleware-name": "path-rewrite", "config": {"expression": ".*", "replacement": "/"}}' http://127.0.0.1:2379/v2/keys/skipper/filter-specs/cut-path
curl -X PUT -d 'value={"middleware-name": "path-rewrite", "config": {"expression": ".*", "replacement": "/pdp"}}' http://127.0.0.1:2379/v2/keys/skipper/filter-specs/pdp-path
curl -X PUT -d 'value={"middleware-name": "path-rewrite", "config": {"expression": ".*", "replacement": "/catalog"}}' http://127.0.0.1:2379/v2/keys/skipper/filter-specs/catalog-path
