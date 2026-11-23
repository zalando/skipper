/*
Package nitro provides TLS certificates whose private keys are held by
ACM for Nitro Enclaves.

The private key never leaves the AWS Nitro Enclave. Skipper terminates
TLS itself and offloads only the asymmetric private key operations
(signatures in the TLS handshake) to the enclave. The enclave exposes the
keys as PKCS#11 tokens through a p11-kit RPC server reachable via vsock.
This package implements a minimal pure Go p11-kit RPC client, so no CGO
and no PKCS#11 shared library are required.

The implementation is only compiled with the build tag "awsnitro":

	go build -tags awsnitro ./cmd/skipper

Without the build tag NewProvider returns ErrNotSupported.

Certificates are registered per host in the certregistry with
certregistry.PriorityNitro, such that they take precedence over
certificates from other sources. The certificate chain files are watched
for changes to support ACM certificate rotation.

The p11-kit RPC protocol is implemented following
https://github.com/p11-glue/p11-kit/tree/master/p11-kit (rpc-message.c,
rpc-client.c, rpc-transport.c).
*/
package nitro
