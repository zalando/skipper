FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:5bafe0881393b6638069f494b472cc27aec88b04a5777c041519b651181091e0

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
