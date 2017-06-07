# Packaging

This directory contains additional scripts and artifacts for packaging Skipper for different platforms or
package managers. (Currently, only docker.)

## Docker

Use the provided Dockerfile to build a docker image with Alpine Linux and the latest version of skipper and
eskip.

Use the `VERSION` and `REGISTRY` or `IMAGE` environment variables to set the docker tag.
`VERSION` defaults to the hash of the current git head, which revision is also used to build the packaged
binary of skipper and eskip.

**Example:**

```
REGISTRY=my-repo VERSION=latest-SNAPSHOT make docker-build docker-push
```

The above command will build a docker image with a tag 'my-repo/skipper:latest-SNAPSHOT' and push it to
my-repo.

**Test:**

Create some artefacts to run skipper in the image:

```
echo '<p>Hello, world!</p>' > hello.html
echo '* -> static("/", "/var/skipper") -> <shunt>' > routes.eskip
```

Verify the routes:

```
docker run -t -v $(pwd):/var/skipper my-repo/skipper:latest-SNAPSHOT eskip check /var/skipper/routes.eskip
```

Run the image:

```
docker run -d -v $(pwd):/var/skipper -p 9090:9090 my-repo/skipper:latest-SNAPSHOT skipper -routes-file /var/skipper/routes.eskip
```

Test the image:

```
curl localhost:9090/hello.html
```

WARNING: the primary use case for this docker image is to have a quick'n'dirty skipper available quick. We don't
necessarily update this image or the Dockerfile, so it may miss some important security updates. Use it at your
own risk or, better, build your own image using these tools.
