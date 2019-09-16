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

Create some artifacts to run skipper in the image:

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

```
Copyright 2015 Zalando SE

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
```
