FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:115dcf2d907a62cc2723b323e1f821b28e6e6bb5465b3065d7ce05f55fa2aaac

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
