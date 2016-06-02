#!/bin/bash

#
# Generates a self-signed server certificate that can be used by
# servers wishing to serve HTTPS requests.
#
# 
# The script generates the following files:
#  - key.pem: the server's private key.
#  - cert.pem: the server's certificate.
#
# NOTE: relies on openssl being installed.
#

sample_subject="/O=Watcherd/CN=Server"
if [ $# -lt 1 ]; then
  echo "error: missing argument(s)" 2>&1
  echo "usage: ${0} <server-subject>" 2>&1
  echo "  <server-subject> could, for example, be '${sample_subject}'" 2>&1
  exit 1
fi

server_subject=${1}
destdir=$(dirname ${0})

# 1. Create a private key for the server:
echo "generating server's private key ..."
openssl genrsa -out ${destdir}/key.pem 2048
# 2. Create the server's self-signed X.509 certificate:
echo "generating server's certificate ..."
openssl req -new -x509 -sha256 -key ${destdir}/key.pem \
   -out ${destdir}/cert.pem -days 365 -subj ${server_subject}
