# Skipper

Muzz plugins live in `plugins/filters/`

To build the container run `docker build -t muzz-skipper .`

## Teapot Plugin

To locally test the Teapot plugin, you can run the following command:

```shell
aws-vault exec dev -- docker \
    run \
        -e AWS_REGION=eu-west-2 \
        -e AWS_ACCESS_KEY_ID \
        -e AWS_SECRET_ACCESS_KEY \
        -e AWS_SESSION_TOKEN \
        -e TEAPOT_S3_BUCKET=euw2-d-all-a-api-gateway-skipper-y5yqa82l \
        -e TEAPOT_S3_SERVICES_KEY=services.json \
        -e TEAPOT_S3_TEAPOTS_KEY=teapots2.json \
        --rm \
        -p 9090:9090 \
        muzz-skipper \
        -inline-routes 'all: * -> preserveHost("true") -> teapot() -> "http://example.com/"; health: Path("/health") -> status(200) -> <shunt>'
```

Then run `curl -v -H 'Host: example.com' http://localhost:9090/`

Inside `teapot-s3/` there are the files that can be synced to S3 to test different parameters.

## Attestation Plugin

To locally test the Attestation plugin, you can run the following command:

```shell
aws-vault exec dev -- docker run --rm \
  -e DYNAMO_TABLE_NAME=d-all-api-gateway \
  -p 9090:9090 \
  muzz-skipper \
  -inline-routes 'all: * -> preserveHost("true") -> attestation() -> "http://example.com/"; health: Path("/health") -> status(200) -> <shunt>'
```

To get a challenge response, run the following
```shell
curl -vsL \
  -H 'Host: example.com' \
  -H 'UDID: 4FD061D3-7936-4646-B53E-77A45277F2FA' \
  -H 'User-Agent: MuzzAlpha/7.51.0 (com.muzmatch.muzmatch.alpha; build:7688; iOS 16.6.1) Alamofire/5.6.4' \
  -H 'appVersion: 7.51.0' \
  -H 'Content-Type: application/x-www-form-urlencoded' \
  -H 'features: SUPPORTS_CHALLENGE_RESPONSE' \
  --data-urlencode 'emailAddress=mark+1@dev.muzz.com' \
  --data-urlencode 'UDID=4FD061D3-7936-4646-B53E-77A45277F2FA' \
  --data-urlencode 'verificationCode=160893' \
  -XPOST "http://localhost:9090/v2.5/auth/confirm"
```

To get a challenge response, run the following
```shell
curl -vsL \
  -H 'Host: example.com' \
  -H 'UDID: 4FD061D3-7936-4646-B53E-77A45277F2FA' \
  -H 'User-Agent: MuzzAlpha/7.51.0 (com.muzmatch.muzmatch.alpha; build:7688; iOS 16.6.1) Alamofire/5.6.4' \
  -H 'appVersion: 7.51.0' \
  -H 'Authorization: Integrity 7VgJVBPXGk4mQ9gioCgIbuNSZA13MgkkUJUUERARZJHFCg6ThWg2k0HE1kqionBKa12qtLaCqBW1Yl1oFYFXLc_dVsXW5SGlKmIfVG2pipb67hDA4LPbOT2v57zTnNy5-W_-u-T_v-___5tySqGhdaRer5YHwCdJ03IjrYRdIq2hN1MLRZQ5DbkLzEgbbIZSDsJGEC67aFmZ4xngwLXzLYg6OwHlICAOHwtGcznJKIfnKWVWw-ATk3YvR9IqnRYLl2I47goGMkoOPAeLUrSW4vcO2vEcwkm1SqEzaFUkGOk-QEAACQ5woUAowNN7RKJHBKa38GgQadkxTCTHASGWibIkQUEyhSALEIRcIQwWkgQlIBWyYFwmFokUQABwMS4DQTI5FQRIiSA4SyAAgAQE7gmGMks58gZKpVIsXG6gVQoV1X3wP3DmNODKte02CcLusQ2Hbct5iYU2T2xK8Dzgduj7iyF3P508-7x_0d4Hre7ml7FIdFDllyk-FfPrc2JO-r5dUI3dKBE06zcuT-VNq-AFs0Js53zB2_Sw3HQfmH4APLjnCFc2-wmKABZwYiRnRoIeQO8CUz7XHm5a8ICS2dmgRjB_C3SXQ10hgB27rhCHHauuUGCRCEsn9Ea9RLEJKVFigpgawad0Gr4mZ5GGpKnspx9ItT6b3MpFUeM8I1ZXGASn2tQVBlvWk1i6EKYDw56ewBZ1Ary6ooV2KBcX8oV8ASCefomgXmBc2RgUi5hkOt3pTJaU2R-b7zbyeGROqFtWwXrR7PU3qJkjR4_63BpknGwWkCMgu6Ux7nHGuMwE9wfNidKuCNS94thB6sFjn8Ilc_elV7MXv9eYGN-xsnCFKJnbkl9LITir9R9RL34TUKzSNlz38Ll21n_xYdcM9fBYm-avZ611OFZJfFcqzvJwkPK_Ozokk7djpnMaEgnhHg7M7OMWyLvYH6pvPhzmebkhcd1hsaddktW5OCABh7_GAsURzwd_gk5HQwb8ASwx-AcQmWJcTEhEIoh_ghHhg3ml__mEWwBcnoLXxs-EssZwsljo7lnBpbmxeXtFb4bdOve9l9e9I5l33PMDvt1nnpu2Iu7h-Q-HvLDwnc0K1GNQVNWLB5Xc9zdcoGoygp3rSt8QvJ9vrFh-i9aWp22-r_O5KqiMyBjtlnpRWfm-12t7ydgjnoqzJeUKIAOD-nBtB7iwY6A0ihkbiw4FbvmDd73lkkjU1GZLn3zAq7j56NubN4vKwAhGwQl1QwdPvD57OOpRcWI__nAZ_WVzc4rh-rv92cHmWjuMo2IBBYRFde3KYmOxDfL54S6Fxj4lXnT4yxX-k5aBaC-f9nOdg1vzSoa_03pg98ML25OIB1UDuHDKTnH7F6dN7awtCUcPTe5cQ8kTfz7ts4S_v2lTRm361JNVJS_HmAd0omLdQtvq1LWSj5UGOSVX6ek0pyzQS88BbFukNB_kQwLizmAA135OQRRbzkEhq1jWSuzS_HH5qJnTipvREYDP4JDN_r10hfh9hLAZkqPmvwP47zzz3wH8_ySAA2-EjTLYx1bwp8efLVl1aY-U4EWeaDw3rG1efMht8dySK8uX3a4e9BmYA23BaKbGRk2VSYSGZKFSlTYlUBmdNyOKjAkXRsULDMoF8mRNnEoar5YvSs2ZlzgzWxsjnqbG4_OCg7SSmdGKvFxp4NxwuVChShQqQUR2Skx8unJ6oF_kjAkTgBMEE7MDV5qUFJGYBJwRGACgbGsktbIs3UIwCmHzmAFU4i4AAiIASAJwPAkXhgjgG-cLJemMyhBGxaKACwJw0E-B1f2CUcXM-RC2pJ60tfhMyYyA1xefcXO85iQ2LXqpH-VfxQHgWwg4vo_y6h7WQJbQcqWhj_oiLACLxHsTnWNvouvHNEyaQ2frDCo67_mccwT2zCDXFUlO7IkPBBDjIoEED2bigxAIQVC3GJQO0vEgILScLsD6XNYhaYqBzJFhCZYQiyWqlFqVVvk79v5lnl_Z4K28M_u8z65hNbUb59u3T_oqZqtH3tA9k9WegsCWmSnRbZ0fVV9EjYi6Ze2lEReO2zWaXxu579Tje12-Pt8scekqN7MvwbKh4RmmWyezy-4_xyhXiWNWd1w9ezElf7DjmxNOgHCunR-XbWNjC30cDESQaj0yYBeMz6ZpfUhgoI4y6vndpTpD824REAEkCS0jUsI4C_cdzuyDoWa2KxScYXPso6wNG5i6gOlo78IIAkxVPNO-BLlaRWopOQbtSWerjBjV51I5lpWHkdo8TE8aaPjJaMzRyI0YSVFyPW2Zo4Bz5EzTYqTFSVnQ4tA_WhlpkGG03KCBE7QyjNJpZSrGaUZmUo5R7t9vI70Ozs2zaPaDlN5AUrSK6l6TlmvkWtrIB6K-H8F28-mxTm5urpVxrNYme0FpVTFMu_fBJheyyFMiO61puRpqm7V-QU3_cGybD5z7bMdzRhGYm61DWSQLTEawRVdl3-RsjLNxe-IfffWHVf6BA_235b4Rb3agl27cFfpuG4JFiI7N8RWExXb6mpQ_ra8yfG1f0uBT9PYtm1Pt9nr3-GKYlTthW9JD2JmPlzb4PSk-Glwy9kD6bm1NvzpTiQ8DHhZKDLJAu6euZLhJ_LncxCWAEAjwYBFBEEzxKWTE3uLzr40cv8ze1WepmsUFH-30dggd3lz5GF2b_9au-3jTtlPeIt3kmObRN1MaBxUXHzK_uq42-lEtcXL3GdTjXmD4C486avedWvraoXLTA2DqgI7v5a4N4MCuH32r91wrSyVWvrOlqORabc2_K6-cGrcTTLGibwgQgyAr-vr-On2ZAQN0I0UqCRDMbDQKhdYF_qW-pd4FXj2TKYPaam6_SXz4nRW0nxteni2GXbgOPdDmIpxnsM2xpGmcVazbtqns6xtts6Ybi1YvmbAwccObHVdGpY48mjP1Z_H2ab5LPT13DNgm0zQfXNN6LnlaOwKUof77m1uDbXNPXWxbsN0zJDl0kmju8fqW1Xuaydr8aNWNTL8KdfNAZMdp8rL4Y-PUnkvWCQv47QLqf1p5vj5m3V8HepiBCKbGlIAgBvQSK_F_d5BfuIltuB04IcywpX3NGIPJdXijy7p79eXT5_9rsNtlp5vbbHxzZa9kThdfqB4--cCSH0dVCbJmn7i9bFYciBuYzupKTeVt_dHZP9Y20jXnyvr163X4y-1-772ChV46mHQ5o2RLyAwHvPwlEGaFo-fi_L-Y8Wu3rB4YLb396WduGZ6zhUMvt9_pUDuxpsR80rR3ysB6566WRa-XbZt39ginYeOPa_5JfzK3acfYIQjQKIpcTjYMaIg7Uxw-cX8RtZXQXtdccf9YNup4olfnyixa4ug-69SM7bzWoZxbm_d6s1g4k9rgfQfmuFV_bXR6Ts31zA3PKougkSACGc26Fz-m6fs7DynegSltTTUn6ybwBFfWrku-qmdleujGD9yPYF7Rg64FAFVorN-NYRvM0-KLHjfe361b0tg4a92r8icbOJbyL5tJdZNJmkzdUnWy4MzqzEerYp123n2j6PXFaQ_Ede_6Nt7AlMda5t7s6Apj9MneP_xk8gVytU7PwjJcxBcPKpr8OBM76PIjlV-NWJ6JsE1faNrmVIzfizlwtrIRjhfGHp2K_daFaEwq9lt3ov8A' \
  -H 'X-KeyId: XhA41blm3ysDPvR0o8Kv1x2FXwIBgdBt7GCpJ7IgCgM=' \
  -H 'X-Assertation: omlzaWduYXR1cmVYRzBFAiEA_DKiduPNY7MnN-5gaLZBzpNgbPf41fSxysCtYSN1N6UCIBtf4UNijn379m3n6M_edzoblI_COHUxApC0KHy2WRLlcWF1dGhlbnRpY2F0b3JEYXRhWCW2yobNkl_6kE0Oq_COiox9Wfc4v5sq3eQgZ8fmauX0_UAAAAAB' \
  -H 'Content-Type: application/x-www-form-urlencoded' \
  -H 'features: SUPPORTS_CHALLENGE_RESPONSE' \
  --data-urlencode 'emailAddress=mark+1@dev.muzz.com' \
  --data-urlencode 'UDID=4FD061D3-7936-4646-B53E-77A45277F2FA' \
  --data-urlencode 'verificationCode=160893' \
  -XPOST "http://localhost:9090/v2.5/auth/confirm"
```
