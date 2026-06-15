FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:72f83785e61158ad0fe0a4f571dfc238aebe2f1c731a4ba5d857e24a2350caa9

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
