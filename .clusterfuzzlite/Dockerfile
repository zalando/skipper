FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:0b3b15cf15170daa8bf6d19c280c10b8b5dc377b266860751789d5603cc2ab43

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
