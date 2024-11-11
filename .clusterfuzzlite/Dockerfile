FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:9a2a2329c2a45a003d1550f0f003dd81cd0816c522ac6b99a0f4b6131f699320

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
