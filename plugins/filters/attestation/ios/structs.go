package ios

import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/binary"
	"github.com/fxamacker/cbor/v2"
)

type AttestationRequest struct {
	RootCert           []byte
	DecodedAttestation []byte
	ChallengeData      []byte
	DecodedKeyID       []byte
	DecodedAppID       []byte
}

type Attestation struct {
	req *AttestationRequest

	credCert        *x509.Certificate
	attestationCbor *attestationCbor
	clientDataHash  [32]byte
	generatedNonce  [32]byte
	publicKey       *ecdsa.PublicKey
}

type attestationCbor struct {
	AttAuthData appleAuthData
	Fmt         string           `cbor:"fmt"`
	AttStmt     appleCborAttStmt `cbor:"attStmt"`
	RawAuthData []byte           `cbor:"authData"`
}

type appleCborAttStmt struct {
	X5C     [][]byte `cbor:"x5c"`
	Receipt []byte   `cbor:"receipt"`
}

type AuthenticatorFlags byte

type AttestedCredentialData struct {
	AAGUID       []byte `cbor:"aaguid"`
	CredentialID []byte `cbor:"credentialId"`
	// The raw credential public key bytes received from the attestation data
	CredentialPublicKey []byte `cbor:"public_key"`
}

type appleAuthData struct {
	RPIDHash []byte                 `cbor:"rpid"`
	Flags    AuthenticatorFlags     `cbor:"flags"`
	Counter  uint32                 `cbor:"sign_count"`
	AttData  AttestedCredentialData `cbor:"att_data"`
	ExtData  []byte                 `cbor:"ext_data"`
}

// AppleAnonymousAttestation has not yet publish schema for the extension(as of JULY 2021.)
type AppleAnonymousAttestation struct {
	Nonce []byte `asn1:"tag:1,explicit"`
}

type appleCborCert struct {
	CredCert []byte `cbor:"credCert"`
	CACert   []byte `cbor:"caCert"`
}

func (a *attestationCbor) UnmarshalCBOR(data []byte) error {
	type attestationCborAlias attestationCbor

	aux := &struct {
		*attestationCborAlias
	}{
		attestationCborAlias: (*attestationCborAlias)(a),
	}

	if err := cbor.Unmarshal(data, &aux); err != nil {
		return err
	}

	a.RawAuthData = aux.RawAuthData
	a.Fmt = aux.Fmt

	a.AttAuthData.RPIDHash = a.RawAuthData[:32]
	a.AttAuthData.Flags = AuthenticatorFlags(a.RawAuthData[32])
	a.AttAuthData.Counter = binary.BigEndian.Uint32(a.RawAuthData[33:37])

	a.AttAuthData.AttData.AAGUID = a.RawAuthData[37:53]
	idLength := binary.BigEndian.Uint16(a.RawAuthData[53:55])
	a.AttAuthData.AttData.CredentialID = a.RawAuthData[55 : 55+idLength]
	a.AttAuthData.AttData.CredentialPublicKey = a.RawAuthData[55+idLength:]

	//minAuthDataLength := 37
	//remaining := len(a.RawAuthData) - minAuthDataLength

	// Apple didn't read the W3C specification properly and sets the attestedCredentialData flag, while it's not present for an assertion. We'll just look the length...
	//if len(attestation.RawAuthData) > minAuthDataLength {
	//	a.unmarshalAttestedData(attestation.RawAuthData)
	//	attDataLen := len(a.AttAuthData.AttData.AAGUID) + 2 + len(a.AttAuthData.AttData.CredentialID) + len(a.AttAuthData.AttData.CredentialPublicKey)
	//	remaining = remaining - attDataLen
	//}

	//if remaining != 0 {
	//	return errors.New("leftover bytes decoding AuthenticatorData")
	//}

	return nil
}
