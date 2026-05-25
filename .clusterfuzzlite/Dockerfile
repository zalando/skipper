FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:3c453ef34e91fe1bfead327926beb428d032cea39d331edd987e1b6f938a9012

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
