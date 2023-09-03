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