FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:a09e5feac586bf8364362ee86d3df8e9ed83fdbf237b1754c7b69a756229798b

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
