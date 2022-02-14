#!/bin/bash

if [[ $# -eq 0 ]] ; then
  echo 'Certificate name required'
  exit 1
fi

openssl req -x509 -sha256 -nodes -newkey rsa:2048 -keyout "$1.key" -out "$1.crt" \
  -days 3650 \
  -subj "/C=DE/ST=Berlin/O=Zalando SE/OU=Technology/CN=do-not-trust-test-data"
