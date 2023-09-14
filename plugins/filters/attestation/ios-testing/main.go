package main

import (
	"crypto/md5"
	_ "embed"
	"encoding/base64"
	"github.com/zalando/skipper/plugins/filters/attestation/ios"
	"log/slog"
)

//go:embed Apple_App_Attestation_Root_CA.pem
var appleRootCertBytes []byte

// Challenge that was issued to the client
var encodedChallengeData = []byte(`12UmVYHgKuH1KPEi-kVPXt2lrDLl2CZiym-uCDdJa5bn9WYnBWSiSxD592hMux6ivEHsMbdb_eSAsGAnpiTaof9UEiS1Ur8h0w5PZYl42RFZtMQFOc6ePXBFkSJAkX4qp2Eb74vvpYP7gXAvDUD4-7IJzYjj6erXLZU6lPA-X9A=`)

// This comes from the device and is sent as the header - it is ZLIB encoded
var encodedAttestation = `7VgJVBPXGk4mQ9g3QVCKMi6lgIB3MglJ8FFFEAQEFFAC1mWYLESzmQwitCoJfShU3C2gVlHUqlite93gVMurivKqYhWtWlpB2spRW5ciIu8OAQw-u53T83rOO51z7tz8N_9_783_f99__5sKSq6mtaROp5IFwTdJ0zIDrYBdMq2mN1PzBJQpDWkHJuQJbAvKOQgbQbjsotW-eZOBHdcmoGB8XRjKQUAiPgwM4XImoxwHr3BmNgy-sfCu6UhaqdVgEeEYjrsBV0bJzsHOrBSjoYJ7Bm0c7CJIlVKu1WuUJBjs6cgjgBgncD6fzxOkQ5EPhLgA4HwRFIFxBR4Dos0rjhEL5YSAh8tgE4h4IVJcyOOLpCKCkknFUrmcoPhCmUiISzNIXMAX84A8hKIIAAf5Ar6UkAlwLzCAmcrewTU8PByLkOlppVxJdW38D-w5DbhxrbtcgrC7fcNhW3PGstDT-fdHl_rzPhrSeSTM9orjypDM---9xc_JL73CI_kHho9N2HDWurLR9Vnja5K966mz77utSw2xf-yT9tVn1c42z2zdK0xsNjA-Bw5w0UFubHYnigAWcGIkZ0aCIUDvA2Me1xauWvCEktpYoQYwZwuMl111IYAdu7oQhx2rupBnlghzx_dDfQXxSanjRQQROy6Y0qqD1Vm5apKmMl98IFW6THIrF0UNsw1YdWEINLWqLhSa5xObu1CmA74vdmCNDgSe1UXzbFAuHhIcEoxXF71tjVrxQLQIB8QLPQT1BcM3DUWxJQn2HgUnZ05JdS1B-723aUFH6r2JS8IKqG2twk73gvdsLAHHUbCAFAGTlmsPLzu1N_SDGZtKPWPaF0fXcdOnJLLnf6EevDShtHLj6bUrjPt-nFZT9Pze8fEVmn0IABdXBHgTj0bepc_nnPW_wBPdftJPy7-zwGmtriX7aWfawfTH23VlM1e1P_Ip3PXp8pNpSDREfgQwsU-b0e9ie7Sm8fgYr4b65DXHRV42KRbb4oAkHP4YMyoHvZoHSVotDcnwB2DFUAEAAhfhIkIsYKhAMCJ8MU_6n8-9ucDlBY6tRhhR1lBOBgvdPVVYnh2fs0-wbMydCz_6-j44OeOeZ17Q9_tNs9IWJf588aP-r89bu1mODuw3_tA_jii4G0ovUSemC52ry5fyNuQZdvzzDq2pSNv8WOt_nbdn3PQhHpLLij0bfBfsI-NPesnryirkQAr69SLcBnBhx4DKhxkbhg4AHnnuu1a4JBMnqjLDOz902NH09PumpqJNYBCj4IR6oO5vfjvNGx2448wB_Od36S8bG1P1367ryxM21zJgHCULyBGcdaxqcbGh2Ar59_EOudo2daLg-JeLAke_C2J8_VsvtLm35JR5r205uPvnS9tTiCeHHLnQpFLU-sU5YytrS9Kpo5FtqyhZ8vNz_guDD9zaOL0qPfbsobK34kyObahIO8_6mGS1-LBCL6NkSh2d5qQDPUR1ZFsj5XkgD1IRdwaOXNuZBePZMg4K-cWyVGKX5w3PQ02cFtyEBoBgBods9u8lLjBxrBA2Q3fU9Hcu_517_juX___lcuCHsFGGBtjyu245TqZ7tYofdha2Xc6uXeR8ZV_CvcQ2N_dbBwPzmn3BTOgVRlMyOUMXFT1LkKDPlaRkSIQJs8PFCbmSaIHQEC4A4bJEQWR2fEIGTxUbq-QrJqYYxs0G2fKoGG3uLHWOjpIAMkQ9OUETky5RkbEZE8ap5iXJZZLssDDgBHHFrMANT0kZl5wCnBGYC6BsbSA10gw0SjsPYAjbgRkawAM8IgiIg3B-Cs4P5fNDeYJgkYCfzmj0f6GB84Jwoq8Gq-uBKcbE-Qi2lO4zbP75sklBS-af97C_6SQy5o7tw_93cACCzWx8o5f_qm4KQcrQMoW-Nw8IsCAsGu859ex7Tr0-tMPCs-hMrV5J57yagPbAlhnkuiGTk7uTBQFEuIAnxoXmZMEHIV1iSDpIx0MA37y7IMt9WeanKD2ZJcWSzPkWS1YqNEqN4nes_cukv1bqp7g37aL_rtdOVK2fY9s6-krc1oE5Az6OVHnxRjZPSY2527b32GXUgKiaV18ddOm0zQ3TgsH7a9sfdAT4f7PQpQOS_iqsIepfYr3lydbg-TxOsVwUt_Lh9brLqXnu9svCzoAIrs0ILtvKyhoGWQgEkGvdMmAXvJFJ07rQkSO1lEEX3FXCM5TvEgERRJLQMwIFDnC4rjezDoaa2G5QcIbNvpezVjAZdQDjqZ6JEQQYDzkY9yfJVEpSQ8kw6E86U2nAqN6QyrCMHIzU5GA6Uk_DTwZDllpmwEiKkulos40c2siYpsFIc5AyoMdhfDRSUi_FaJleDQ00UozSaqRKJmgGxijLIAvss5BOC21zzJp9IKXTkxStpLrmpGVqmYY2BANB749ge_h3eyc7O9vCORZzkz2gtCgfJjz4cKMLWeQllp5TN18fZZ1RMvdE39RsnQece33n4Iwi8KC2zGXRLBCJYLnXpd9krU-08ugMjLn-0_LAka6B27KXTjTZ0fnrd41adxfBxgk-nxnAGxPfFmBUPCs5pP_atqzev-j9O1a1rbY6z4nF8Ihug21hN2GntOfXj-gsPiUsG3YwfbfmRJ-iU4G_BgaaKdHPDO3uIpPhJvHnchMXA4LHw4UCgiCYSpTPiD2V6F-bOX6ZvSvrqBPzC_ZW-tmN8m7c046uzlux6zF-a1utn0AbGdc4pCn1Rr_i4qOmd9ZUxTytIs7uPo8OfDAy4vWnD6v21-YvOFphfAKMD2Hge7hrBTiw60PfYx_f3CQhFq_dUlR2s-rED3uu1Q6vBFEW9A0FIhBiQd-AX6cvM6CHYaRIBQGEzEI-KPQuCCwPKPcr8O02pvQqC9s-RsHwOwtovzK9vFwZu3DtuqHNRTgvYZvDyWQBGSx8i7XbNm76-vbdqQmGopULw-Ylly57eM1HMvhUVuxz0fYJAfleXjsdt0nVjUdWtVyYPKEVAYpRgQcaW4TW2bWX787d7hU6edRowazTNc0rP24kq_JilLdnjNihanRFdp4jG0SHDbHdN64zZvDbBNU8W3yxJm7NXwd6eAIRAF7AxCCEAb3YQvzfbeQXrmWl340MG6Pf0rpqqN7o5n3DZc2DmoqEOV-5ezQ4NW2zCsiWvj0jQXTpmHfkwYWPfA7xMqad-e7dqYkg0TWd1SGROGx95BwYbx3tlnWtpKREi7_VOuKDt7FRV4-kNEwv2xI6yQ6vGAvGWODolTj_L2b82pWrG0b53336mcd0r2n8AQ2t9x6qnFhRcZ_c2hflWuPc0Zy7ZNO22XUnOfXrH636F_3JrFs7h_VHgFpe5HK23rE-8XxxxJsHiqithOZb9TXPw1Kf08m-bYszaLG959TaSdsdWgZw7mze58di4bDONrYjTMG9_K_NTq-ouV667lmcIuh4EIUMYRVMX3P-6jdj_fi0Z1nCWq8d3nfdLybbr02YxlWdpL-wroEqFV_NnPrhDAX3Zpzfl4WVaRcjTbI5nS1HJrzulDzzqJGTb67_MpmzLpKkScmWQ2cLzq-c8XR5vFPl_aVFS-anPRFVrwu4cRtTfN48q-lhxxhGn-z5J1AqmytTaXUsrOSnpMFJ_U1qz-HF-XUNt376bMGiQUf7x2CPz4Ho2ymXmrayEY4vxh4iwX7rejRUgv3WDek_`
var encodedKeyID = `l/NSHlIVgm0XJI2Dztnf88R+hx26FUkg9swwR+RU0+U=`

func buildRequest(
	appleRootCertBytes []byte,
	encodedAttestation string,
	encodedChallengeData []byte,
	encodedKeyID string,
) (*ios.AttestationRequest, error) {
	var req ios.AttestationRequest
	req.RootCert = appleRootCertBytes

	slog.Error("b challenge data", "v", []byte(encodedChallengeData))
	slog.Error("s challenge data", "v", encodedChallengeData)
	slog.Error("encodedKeyID", "v", encodedKeyID)
	slog.Error("challenge data md5", "v", md5.Sum(encodedChallengeData))

	decodedAttestationPayload, err := base64.URLEncoding.DecodeString(encodedAttestation)
	slog.Debug("attestation payload", "payload", encodedAttestation)
	if err != nil {
		slog.Error("cannot decode attestation payload", "error", err)
		return nil, err
	}
	req.DecodedAttestation = decodedAttestationPayload // Still in ZLIB format

	req.ChallengeData = []byte(encodedChallengeData)

	decodedKeyID, err := base64.StdEncoding.DecodeString(encodedKeyID)
	slog.Debug("key id payload", "payload", encodedKeyID)
	if err != nil {
		slog.Error("cannot decode key id payload", "error", err)
		return nil, err
	}
	req.DecodedKeyID = decodedKeyID

	return &req, nil
}

func main() {
	req, err := buildRequest(
		appleRootCertBytes,
		encodedAttestation,
		encodedChallengeData,
		encodedKeyID,
	)
	if err != nil {
		slog.Error("bad request", "err", err)
		return
	}

	attestation := ios.NewAttestation(req)

	if err = attestation.Parse(); err != nil {
		slog.Error("parse fail", "err", err)
		return
	}

	// Step 1.
	// Verify that the x5c array contains the intermediate and leaf certificates for App Attest,
	// starting from the credential certificate in the first data buffer in the array (credcert).
	// Verify the validity of the certificates using Apple's App Attest root certificate.
	if err = attestation.ValidateCertificate(); err != nil {
		slog.Error("validate certificate", "err", err)
		return
	}

	// Step 2.
	// Create clientDataHash as the SHA256 hash of the one-time challenge your server sends to your app before
	// performing the attestation, and append that hash to the end of the authenticator data
	// (authData from the decoded object).
	attestation.ClientHashData()

	// Step 3.
	// Generate a new SHA256 hash of the composite item to create nonce.
	attestation.GenerateNonce()

	// Step 4.
	// Obtain the value of the credCert extension with OID 1.2.840.113635.100.8.2, which is a DER-encoded ASN.1
	// sequence. Decode the sequence and extract the single octet string that it contains.
	// Verify that the string equals nonce.
	if err = attestation.CheckAgainstNonce(); err != nil {
		slog.Error("check against nonce", "err", err)
		return
	}

	// Step 5.
	// Create the SHA256 hash of the public key in credCert, and verify that it matches the key identifier from your app.
	if err = attestation.GeneratePublicKey(); err != nil {
		slog.Error("generate public key", "err", err)
		return
	}

	// Step 6.
	// Compute the SHA256 hash of your app's App ID, and verify that it's the same as the authenticator
	// data's RP ID hash.
	if err = attestation.CheckAgainstAppID(); err != nil {
		slog.Error("check against appID", "err", err)
		return
	}

	// Step 7.
	// Verify that the authenticator data’s counter field equals 0.
	if err = attestation.CheckCounterIsZero(); err != nil {
		slog.Error("check counter is zero", "err", err)
		return
	}

	// Step 8.
	// Verify that the authenticator data’s aaguid field is either appattestdevelop if operating in the
	// development environment, or appattest followed by seven 0x00 bytes if operating in the production environment.
	if err = attestation.ValidateAAGUID(); err != nil {
		slog.Error("validate AAGUID", "err", err)
		return
	}

	// Step 9.
	// Verify that the authenticator data’s credentialId field is the same as the key identifier.
	if err = attestation.ValidateCredentialID(); err != nil {
		slog.Error("validate credentialID", "err", err)
		return
	}

	slog.Info("Complete!")
}
