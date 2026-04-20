FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:98937db52b1b32d6c0f125ad79737555696ee78770b2f56301be6a1077e4f244

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
