FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:23b10d0313226a2c94135170e4991a88c4067636dfdac0f074a214bdbc235836

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
