FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:fea3ac4074ac94e5f5c2dfc73b4bfb22b15f9071c965faa139349087576c96ef

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
