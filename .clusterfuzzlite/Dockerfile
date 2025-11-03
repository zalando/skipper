FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:9e293085f163de4a9d9c02f0c17663b3d1e082db3355c142144bc2483d69b5ef

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
