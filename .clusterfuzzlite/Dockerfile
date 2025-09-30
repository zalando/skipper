FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:76ed10bc1c0afaade5460f7fd505401a0aeb3adb3a0a38eccfbb2fb8e68b46d6

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
