module github.com/zalando/skipper

require (
	github.com/AlexanderYastrebov/noleak v0.0.0-20230711175737-345842f874fb
	github.com/MicahParks/keyfunc v1.9.0
	github.com/abbot/go-http-auth v0.4.0
	github.com/andybalholm/brotli v1.2.0
	github.com/aryszka/jobqueue v0.0.3
	github.com/bombsimon/logrusr/v4 v4.1.0
	github.com/cenkalti/backoff v2.2.1+incompatible
	github.com/cespare/xxhash/v2 v2.3.0
	github.com/cjoudrey/gluahttp v0.0.0-20201111170219-25003d9adfa9
	github.com/cjoudrey/gluaurl v0.0.0-20161028222611-31cbb9bef199
	github.com/coreos/go-oidc/v3 v3.17.0
	github.com/dchest/siphash v1.2.3
	github.com/dgryski/go-jump v0.0.0-20211018200510-ba001c3ffce0
	github.com/dgryski/go-mpchash v0.0.0-20200819201138-7382f34c4cd1
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f
	github.com/dimfeld/httppath v0.0.0-20170720192232-ee938bf73598
	github.com/docker/go-connections v0.6.0
	github.com/envoyproxy/go-control-plane/envoy v1.36.0
	github.com/ghodss/yaml v1.0.0
	github.com/golang-jwt/jwt/v4 v4.5.2
	github.com/google/go-cmp v0.7.0
	github.com/google/uuid v1.6.0
	github.com/hashicorp/memberlist v0.5.4
	github.com/instana/go-sensor v1.72.0
	github.com/lightstep/lightstep-tracer-go v0.26.0
	github.com/miekg/dns v1.1.72
	github.com/oklog/ulid v1.3.1
	github.com/open-policy-agent/opa v1.7.1
	github.com/open-policy-agent/opa-envoy-plugin v1.7.1-envoy
	github.com/opentracing/basictracer-go v1.1.0
	github.com/opentracing/opentracing-go v1.2.0
	github.com/prometheus/client_golang v1.23.2
	github.com/prometheus/client_model v0.6.2
	github.com/rcrowley/go-metrics v0.0.0-20201227073835-cf1acfcdf475
	github.com/redis/go-redis/v9 v9.17.3
	github.com/sarslanhan/cronmask v0.0.0-20230801193303-54e29300a091
	github.com/sirupsen/logrus v1.9.4
	github.com/sony/gobreaker v1.0.0
	github.com/stretchr/testify v1.11.1
	github.com/szuecs/rate-limit-buffer v0.9.0
	github.com/testcontainers/testcontainers-go v0.40.0
	github.com/tidwall/gjson v1.18.0
	github.com/tsenart/vegeta v12.7.0+incompatible
	github.com/uber/jaeger-client-go v2.30.0+incompatible
	github.com/uber/jaeger-lib v2.4.1+incompatible
	github.com/yookoala/gofast v0.8.0
	github.com/yuin/gopher-lua v1.1.1
	go.opentelemetry.io/contrib/exporters/autoexport v0.65.0
	go.opentelemetry.io/contrib/propagators/autoprop v0.65.0
	go.opentelemetry.io/otel v1.40.0
	go.opentelemetry.io/otel/bridge/opentracing v1.40.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.40.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.40.0
	go.opentelemetry.io/otel/exporters/stdout/stdouttrace v1.40.0
	go.opentelemetry.io/otel/sdk v1.40.0
	go.opentelemetry.io/otel/trace v1.40.0
	go4.org/netipx v0.0.0-20231129151722-fdeea329fbba
	golang.org/x/crypto v0.47.0
	golang.org/x/exp v0.0.0-20251209150349-8475f28825e9
	golang.org/x/net v0.49.0
	golang.org/x/oauth2 v0.35.0
	golang.org/x/sync v0.19.0
	golang.org/x/term v0.39.0
	golang.org/x/text v0.33.0
	golang.org/x/time v0.14.0
	google.golang.org/protobuf v1.36.11
	gopkg.in/go-jose/go-jose.v2 v2.6.3
	gopkg.in/yaml.v2 v2.4.0
	layeh.com/gopher-json v0.0.0-20201124131017-552bb3c4c3bf
)

require (
	cloud.google.com/go/compute/metadata v0.9.0 // indirect
	dario.cat/mergo v1.0.2 // indirect
	github.com/Azure/go-ansiterm v0.0.0-20210617225240-d185dfc1b5a1 // indirect
	github.com/HdrHistogram/hdrhistogram-go v1.1.2 // indirect
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/agnivade/levenshtein v1.2.1 // indirect
	github.com/armon/go-metrics v0.4.1 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/bmizerany/perks v0.0.0-20141205001514-d9a9656a3a4b // indirect
	github.com/bytecodealliance/wasmtime-go/v3 v3.0.2 // indirect
	github.com/cenkalti/backoff/v4 v4.3.0 // indirect
	github.com/cenkalti/backoff/v5 v5.0.3 // indirect
	github.com/cncf/xds/go v0.0.0-20251022180443-0feb69152e9f // indirect
	github.com/containerd/containerd/v2 v2.1.5 // indirect
	github.com/containerd/errdefs v1.0.0 // indirect
	github.com/containerd/errdefs/pkg v0.3.0 // indirect
	github.com/containerd/log v0.1.0 // indirect
	github.com/containerd/platforms v1.0.0-rc.1 // indirect
	github.com/containerd/typeurl/v2 v2.2.3 // indirect
	github.com/cpuguy83/dockercfg v0.3.2 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/dgraph-io/badger/v4 v4.8.0 // indirect
	github.com/dgraph-io/ristretto/v2 v2.2.0 // indirect
	github.com/dgryski/go-gk v0.0.0-20200319235926-a69029f61654 // indirect
	github.com/distribution/reference v0.6.0 // indirect
	github.com/docker/docker v28.5.1+incompatible // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/ebitengine/purego v0.8.4 // indirect
	github.com/envoyproxy/protoc-gen-validate v1.2.1 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/go-ini/ini v1.67.0 // indirect
	github.com/go-jose/go-jose/v4 v4.1.3 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/gobwas/glob v0.2.3 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/btree v1.1.3 // indirect
	github.com/google/flatbuffers v25.2.10+incompatible // indirect
	github.com/google/pprof v0.0.0-20250630185457-6e76a2b096b5 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.27.7 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-immutable-radix v1.3.1 // indirect
	github.com/hashicorp/go-metrics v0.5.4 // indirect
	github.com/hashicorp/go-msgpack/v2 v2.1.5 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/go-sockaddr v1.0.7 // indirect
	github.com/hashicorp/golang-lru v0.5.4 // indirect
	github.com/influxdata/tdigest v0.0.1 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/klauspost/compress v1.18.0 // indirect
	github.com/lightstep/lightstep-tracer-common/golang/gogo v0.0.0-20210210170715-a8dfcb80d3a7 // indirect
	github.com/looplab/fsm v1.0.3 // indirect
	github.com/lufia/plan9stats v0.0.0-20211012122336-39d0f177ccd0 // indirect
	github.com/magiconair/properties v1.8.10 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/mattn/go-runewidth v0.0.16 // indirect
	github.com/moby/docker-image-spec v1.3.1 // indirect
	github.com/moby/go-archive v0.1.0 // indirect
	github.com/moby/locker v1.0.1 // indirect
	github.com/moby/patternmatcher v0.6.0 // indirect
	github.com/moby/sys/sequential v0.6.0 // indirect
	github.com/moby/sys/user v0.4.0 // indirect
	github.com/moby/sys/userns v0.1.0 // indirect
	github.com/moby/term v0.5.0 // indirect
	github.com/morikuni/aec v1.0.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/olekukonko/tablewriter v0.0.5 // indirect
	github.com/onsi/ginkgo v1.16.5 // indirect
	github.com/onsi/gomega v1.19.0 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.1.1 // indirect
	github.com/peterh/liner v1.2.2 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/planetscale/vtprotobuf v0.6.1-0.20240319094008-0393e58bdf10 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/power-devops/perfstat v0.0.0-20210106213030-5aafc221ea8c // indirect
	github.com/prometheus/common v0.67.5 // indirect
	github.com/prometheus/otlptranslator v1.0.0 // indirect
	github.com/prometheus/procfs v0.19.2 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/sean-/seed v0.0.0-20170313163322-e2103e2c3529 // indirect
	github.com/shirou/gopsutil/v4 v4.25.6 // indirect
	github.com/streadway/quantile v0.0.0-20220407130108-4246515d968d // indirect
	github.com/tchap/go-patricia/v2 v2.3.3 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.0 // indirect
	github.com/tklauser/go-sysconf v0.3.12 // indirect
	github.com/tklauser/numcpus v0.6.1 // indirect
	github.com/vektah/gqlparser/v2 v2.5.30 // indirect
	github.com/xeipuuv/gojsonpointer v0.0.0-20190905194746-02993c407bfb // indirect
	github.com/xeipuuv/gojsonreference v0.0.0-20180127040603-bd5ef7bd5415 // indirect
	github.com/yashtewari/glob-intersection v0.2.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/contrib/bridges/prometheus v0.65.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.62.0 // indirect
	go.opentelemetry.io/contrib/propagators/aws v1.40.0 // indirect
	go.opentelemetry.io/contrib/propagators/b3 v1.40.0 // indirect
	go.opentelemetry.io/contrib/propagators/jaeger v1.40.0 // indirect
	go.opentelemetry.io/contrib/propagators/ot v1.40.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc v0.16.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp v0.16.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc v1.40.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp v1.40.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.40.0 // indirect
	go.opentelemetry.io/otel/exporters/prometheus v0.62.0 // indirect
	go.opentelemetry.io/otel/exporters/stdout/stdoutlog v0.16.0 // indirect
	go.opentelemetry.io/otel/exporters/stdout/stdoutmetric v1.40.0 // indirect
	go.opentelemetry.io/otel/log v0.16.0 // indirect
	go.opentelemetry.io/otel/metric v1.40.0 // indirect
	go.opentelemetry.io/otel/sdk/log v0.16.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.40.0 // indirect
	go.opentelemetry.io/proto/otlp v1.9.0 // indirect
	go.uber.org/atomic v1.9.0 // indirect
	go.uber.org/automaxprocs v1.6.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.yaml.in/yaml/v2 v2.4.3 // indirect
	golang.org/x/mod v0.31.0 // indirect
	golang.org/x/sys v0.40.0 // indirect
	golang.org/x/tools v0.40.0 // indirect
	golang.org/x/tools/godoc v0.1.0-deprecated // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260128011058-8636f8732409 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260128011058-8636f8732409 // indirect
	google.golang.org/grpc v1.78.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	oras.land/oras-go/v2 v2.6.0 // indirect
	sigs.k8s.io/yaml v1.6.0 // indirect
)

go 1.25.7
