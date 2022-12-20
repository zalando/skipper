# Remote eskip

Skipper can fetch routes in eskip format over HTTP:

```sh
curl https://opensource.zalando.com/skipper/data-clients/example.eskip
hello: Path("/hello") -> "https://www.example.org"

skipper -routes-urls=https://opensource.zalando.com/skipper/data-clients/example.eskip

curl -s http://localhost:9090/hello | grep title
    <title>Example Domain</title>
```

You may use multiple urls separated by comma and configure url poll interval via `-source-poll-timeout` flag.
