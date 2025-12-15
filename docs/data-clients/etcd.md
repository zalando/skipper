# etcd

etcd is an open-source distributed key value store:
[https://github.com/etcd-io/etcd](https://github.com/etcd-io/etcd). Skipper can use it as a route configuration
storage and continuously synchronize the routing from etcd.

## Why storing Skipper routes in etcd?

When running multiple Skipper instances, changing the configuration of each instance by accessing the instances
directly on the fly can be complex and error-prone. With etcd, we need to update the routes only in etcd and
each Skipper instance will synchronize its routing from the new version.

Further benefits of using etcd are improved resiliency and the usage of a standard configuration storage for
various system components of a distributed system, not only Skipper.

_Note_: in case of Kubernetes, the standard recommended way is to use the [Kubernetes Ingress
API](./kubernetes.md).

## Starting Skipper with etcd

Example:

```
skipper -etcd-urls http://localhost:2379,http://localhost:4001
```

An additional startup option is the `-etcd-prefix`. When using multiple Skipper deployments with different
purpose, this option allows us to store separate configuration sets for them in the same etcd cluster. Example:

```
skipper -etcd-urls https://cluster-config -etcd-prefix skipper1
```

_Note_: when the etcd URL points to an [etcd proxy](https://coreos.com/etcd/docs/latest/v2/proxy.html), Skipper
will internally use the proxy only to resolve the URLs of the etcd replicas, and access them for the route
configuration directly.

## etcd version

Skipper uses currently the V2 API of etcd.

## Storage schema

Skipper expects to find the route configuration by default at the `/v2/keys/skipper/routes` path. In this path,
the 'skipper' segment can be optionally overridden by the `-etcd-prefix` startup option.

The `/v2/keys/skipper/routes` node is a directory that contains the routes as individual child nodes, accessed
by the path `/v2/keys/skipper/routes/<routeID>`. The value of the route nodes is the route expression without
the route ID in [eskip format](https://pkg.go.dev/github.com/zalando/skipper/eskip).

## Maintaining route configuration in etcd

etcd (v2) allows generic access to its API via the HTTP protocol. It also provides a supporting client tool:
etcdctl. Following the above described schema, both of them can be used to maintain Skipper routes. In addition,
Skipper also provides a supporting client tool: [eskip](https://pkg.go.dev/github.com/zalando/skipper/cmd/eskip),
which can provide more convenient access to the routes in etcd.

Getting all routes, a single route, insert or update and delete via HTTP:

```
curl http://localhost:2379/v2/keys/skipper/routes
curl http://localhost:2379/v2/keys/skipper/routes/hello
curl -X PUT -d 'value=* -> status(200) -> inlineContent("Hello, world!") -> <shunt>' http://localhost:2379/v2/keys/skipper/routes/hello
curl -X DELETE http://localhost:2379/v2/keys/skipper/routes/hello
```

Getting all route IDs, a route expression stored with an ID, insert or update and delete with etcdctl:

```
etcdctl --endpoints http://localhost:2379,http://localhost:4001 ls /skipper/routes
etcdctl --endpoints http://localhost:2379,http://localhost:4001 get /skipper/routes/hello
etcdctl --endpoints http://localhost:2379,http://localhost:4001 set -- /skipper/routes/hello '* -> status(200) -> inlineContent("Hello, world!") -> <shunt>'
etcdctl --endpoints http://localhost:2379,http://localhost:4001 rm /skipper/routes/bello
```

We use the name 'eskip' for two related concepts: the eskip syntax of route configuration and the eskip command
line tool. The command line tool can be used to check the syntax of skipper routes, format route files, prepend
or append filters to multiple routes, and also to access etcd.

Getting all routes, a single route, insert or update and delete with eskip:

```
eskip print -etcd-urls http://localhost:2379,http://localhost:4001
eskip print -etcd-urls http://localhost:2379,http://localhost:4001 | grep hello
eskip upsert -etcd-urls http://localhost:2379,http://localhost:4001 -routes 'hello: * -> status(200) -> inlineContent("Hello, world!") -> <shunt>'
eskip delete -etcd-urls http://localhost:2379,http://localhost:4001 -ids hello
```

When storing multiple configuration sets in etcd, we can use the `-etcd-prefix` to distinguish between them.

Instead of using routes inline, it may be more convenient to edit them in a file and store them in etcd directly
from the file.

Contents of example.eskip:

```
hello: * -> status(200) -> inlineContent("Hello, world!") -> <shunt>;
helloTest: Path("/test") -> status(200) -> inlineContent("Hello, test!") -> <shunt>;
```

Updating those routes in etcd that are defined in the file, or inserting them from the file in case they don't
exist in etcd, yet:

```
eskip upsert -etcd-urls http://localhost:2379,http://localhost:4001 example.eskip
```

The above command won't modify or delete those routes, whose ID is missing from example.eskip. To fully sync a
set of routes from a file to etcd, use the reset subcommand:

```
eskip reset -etcd-urls http://localhost:2379,http://localhost:4001 example.eskip
```

For more information see the [documentation](https://pkg.go.dev/github.com/zalando/skipper/cmd/eskip) or `eskip -help`.
