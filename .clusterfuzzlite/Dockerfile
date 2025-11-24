FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:77aa56b58bd7aa4bcc793e04db55fed10d61b60e5821ba0929ffce4d187410f8

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
