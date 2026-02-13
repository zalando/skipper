FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:84431ed7c10f0fcd04dd83723baefcb4fdf274fb13f72311fbb5964dbac01ada

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
