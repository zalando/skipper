FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:b3111d8c1f679c67e40b14fd839c5e1c5d0fc2b5bcb08f42c7ac9323599ce308

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
