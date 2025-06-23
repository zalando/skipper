FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:95d48cc847cc538f592c973c91b4f8d267023b9df1909db893852a6360b1f56d

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
