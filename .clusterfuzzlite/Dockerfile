FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:76e1fd1dd59317ffaee966cc53d0e333a5447c8e06ef63cbb789cdf602576b28

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
