FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:4c12653fe9a91b08b5112496ebd871f42cb258123d77e44175f00965bdf3dd17

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
