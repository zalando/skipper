#! /bin/bash

host=$1
if [ -z "$host" ]; then
    host=http://127.0.0.1:2379
fi

root=/v2/keys/skipper

sc=scompositor
sls=streaming-layout-service
layoutServiceBackend="$sc"

function put() {
    curl -k -X PUT -d 'value='"$2" "$host""$root"/routes/"$1"
}

put header 'Path("/tessera/header") -> xalando() -> pathRewrite(/.*/, "/") -> requestHeader("Host", "header.mop-taskforce.zalan.do") -> "https://header.mop-taskforce.zalan.do"'
put footer 'Path("/tessera/footer") -> xalando() -> pathRewrite(/.*/, "/") -> requestHeader("Host", "footer.mop-taskforce.zalan.do") -> "https://footer.mop-taskforce.zalan.do"'
put pdp 'PathRegexp(/.*\.html/) -> xalando() -> requestHeader("Host", "'"$layoutServiceBackend"'.mop-taskforce.zalan.do") -> pathRewrite(/.*/, "/pdp") -> "https://'"$layoutServiceBackend"'.mop-taskforce.zalan.do"'
put pdpsc 'PathRegexp(/\/sc\/.*\.html/) -> xalando() -> requestHeader("Host", "'"$sc"'.mop-taskforce.zalan.do") -> pathRewrite(/.*/, "/pdp") -> "https://'"$sc"'.mop-taskforce.zalan.do"'
put pdpsls 'PathRegexp(/\/sls\/.*\.html/) -> xalando() -> requestHeader("Host", "'"$sls"'.mop-taskforce.zalan.do") -> pathRewrite(/.*/, "/pdp") -> "https://'"$sls"'.mop-taskforce.zalan.do"'
put catalog 'Path("/<string>") -> xalando() -> requestHeader("Host", "'"$layoutServiceBackend"'.mop-taskforce.zalan.do") -> pathRewrite(/.*/, "/catalog") -> "https://'"$layoutServiceBackend"'.mop-taskforce.zalan.do"'
put catalogsc 'Path("/sc/<string>") -> xalando() -> requestHeader("Host", "'"$sc"'.mop-taskforce.zalan.do") -> pathRewrite(/.*/, "/catalog") -> "https://'"$sc"'.mop-taskforce.zalan.do"'
put catalogsls 'Path("/sls/<string>") -> xalando() -> requestHeader("Host", "'"$sls"'.mop-taskforce.zalan.do") -> pathRewrite(/.*/, "/catalog") -> "https://'"$sls"'.mop-taskforce.zalan.do"'
put slow 'Path("/slow") -> xalando() -> requestHeader("Host", "bugfactory.mop-taskforce.zalan.do") -> "https://bugfactory.mop-taskforce.zalan.do"'
put debug 'Path("/debug") -> xalando() -> pathRewrite(/.*/, "/") -> requestHeader("Host", "bugfactory.mop-taskforce.zalan.do") -> "https://bugfactory.mop-taskforce.zalan.do"'
put cart 'PathRegexp(/\/api\/cart\/.*/) -> xalando() -> requestHeader("Host", "cart-taskforce.zalan.do") -> "https://cart.mop-taskforce.zalan.do"'
put healthcheck 'Path("/healthcheck") -> healthcheck() -> <shunt>'
put humanstxt 'Path("/humans.txt") -> humanstxt() -> <shunt>'
