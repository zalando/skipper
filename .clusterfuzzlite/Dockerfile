FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:1f8967c219ee638e48ffb1e5193988a7c81c433e4c538073926ccec3e411ebc2

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
