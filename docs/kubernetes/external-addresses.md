# External Addresses (External Name)

In Kubernetes, it is possible to define services with external names (type=ExternalName). For Ingress objects,
Skipper supports these services, if enabled by `-enable-kubernetes-external-names`. Skipper generates routes from the Ingress objects that reference one or more
external name service, that will have a backend pointing to the network address defined by the specified
service.

Route groups don't support services of type ExternalName, but they support network backends, and even LB
backends with explicit endpoints with custom endpoint addresses. This way, it is possible to achieve the same
with route groups.

For both the Ingress objects and the route groups, the accepted external addresses must be explicitly allowed by
listing regexp expressions of which at least one must be matched by the domain name of these addresses. The
allow list is a startup option, defined via command line flags or in the configuration file. Enforcing this
list happens only in the Kubernetes Ingress mode of Skipper.

### Specifying allowed external names via command line flags

For compatibility reasons, the validation needs to be enabled with an explicit toggle:

```sh
skipper -kubernetes \
-enable-kubernetes-external-names \
-kubernetes-only-allowed-external-names \
-kubernetes-allowed-external-name "^one[.]example[.]org$" \
-kubernetes-allowed-external-name "^two[.]example[.]org$"
```

### Specifying allowed external names via a config file

For compatibility reasons, the validation needs to be enabled with an explicit toggle:

```yaml
enable-kubernetes-external-names: true
kubernetes-only-allowed-external-names: true
kubernetes-allowed-external-names:
- ^one[.]example[.]org$
- ^two[.]example[.]org$
```
