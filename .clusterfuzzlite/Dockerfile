FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:4235b87b0a119a2d8dcf4855ab51fc8b4a854e7e2529d684f03ef22ca4234aef

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
