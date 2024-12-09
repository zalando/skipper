FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:36809facb3f1d6ad2ccfd579b6e0ca13bea2ed260a289c85c2fe9270bbffcb38

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
