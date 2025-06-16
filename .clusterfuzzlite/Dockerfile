FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:f5703ce04bc8551420ec24c3ecd783aeeab4d351ce9f19e8ab67340fa2caf526

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
