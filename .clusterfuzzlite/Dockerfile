FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:cb7cb93df5966f9020cdf91dceb2d9235d8e450826d806e7d8aa1e49e2fc7ae9

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
