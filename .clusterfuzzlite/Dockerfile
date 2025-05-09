FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:a703fbcb5a3c875ca345b41d37eb232065942ec7a3657ef0ccef140e9cfc8831

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
