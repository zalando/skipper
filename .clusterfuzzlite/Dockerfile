FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:01065ee37db1914ee49a8c9e8f69b4623bb48e5874ed61cd1e0100cf46963d45

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
