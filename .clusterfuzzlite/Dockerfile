FROM gcr.io/oss-fuzz-base/base-builder-go@sha256:5fb49e09e21e0410a857027f083cb235508e5b5a4d9de88218ab843cf67f0175

COPY . $SRC/skipper
COPY ./.clusterfuzzlite/build.sh $SRC/

WORKDIR $SRC/skipper
