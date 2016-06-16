# Packaging

This directory may contain additional scripts and artefacts for packaging Skipper for different platforms or
package managers.

## Docker

Use the provided Dockerfile to build a docker image with Arch Linux and skipper installed from git.

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
