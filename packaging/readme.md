# Packaging

This directory may contain additional scripts and artefacts for packaging Skipper for different platforms or
package managers.

## Docker

Use the provided Dockerfile to build a docker image with Arch Linux and the latest version of skipper.

```
docker build -t my/skipper packaging
```

Create some artefacts to run skipper in the image:

```
echo '<p>Hello, world!</p>' > hello.html
echo '* -> static("/", "/var/skipper") -> <shunt>' > routes.eskip
```

Run the image:

```
docker run -d -v $(pwd):/var/skipper -p 9090:9090 my/skipper skipper -routes-file /var/skipper/routes.eskip
```

Test the image:

```
curl localhost:9090/hello.html
```

WARNING: the primary use case for this docker image is to have a quick'n'dirty skipper available quick. We don't
necessarily update this image or the Dockerfile, so it may miss some important security updates. Use it at your
own risk.
