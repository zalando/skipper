[![GoDoc](https://pkg.go.dev/github.com/zalando/pathmux?status.svg)](https://pkg.go.dev/github.com/zalando/pathmux)
[![Go Report Card](https://goreportcard.com/badge/github.com/zalando/pathmux)](https://goreportcard.com/report/github.com/zalando/pathmux)
[![Go Cover](https://gocover.io/_badge/github.com/zalando/pathmux)](https://gocover.io/github.com/zalando/pathmux)

# Pathmux: tree lookup with wildcard matching

Pathmux is a package that implements an effective tree lookup with wildcard matching. It is a fork of the awesome [httptreemux](https://github.com/dimfeld/httptreemux). This fork makes visible the interface of the internal tree lookup implementation of the original httptreemux package, and with the HTTP-related wrapper code stripped away.

In addition to having the original httptreemux logic, pathmux offers one small feature: backtracking. This means that when a
path is matched, it is possible to instruct the lookup not to return the found object when other, custom
conditions are not met, but instead continue the lookup by backtracking from the current point in the tree.

Pathmux is used by [Skipper](https://github.com/zalando/skipper), an extensible HTTP routing server used in production at Zalando.

### When to Use pathmux Instead of httptreemux

Almost never, except when:

- you want to store a large number of **custom** objects in an effective lookup tree, keyed by path values
- you want to use the backtracking feature to refine the evaluation of wildcard matching with custom logic

### Installation

```
go get github.com/zalando/pathmux
```

Pathmux is 'go get compatible'. The master head is always stable (at least by intent), and it doesn't use any vendoring itself. However, we strongly recommend that you use vendoring in the final, importing, executable package.

### Documentation

You can find detailed package documentation [here](https://pkg.go.dev/github.com/zalando/pathmux).

### Working with the Code

While it is enough to use `go get` to import the package, when modifying the code, it is worth cherry-picking from the tasks in the provided simple Makefile. 

### Contributing
See our detailed [contribution guidelines](https://github.com/zalando/pathmux/blob/master/CONTRIBUTING.md). If you plan on contributing to the current repository, please make sure:

1. to run the precommit checks:

```
make precommit
```

Please fix all reported errors and please make sure that the overall test coverage is not decreased. (Please
feel free to provide tests for the missing spots :))

2. compare the performance of the new version to the current master, and avoid degradations:

```
make bench
```

(Comparison currently can happen only by running the same `make bench` on the master branch. Automating the comparison would be a nice contribution.)

### License

Copyright (c) 2014 Daniel Imfeld, 2015 Zalando SE

Permission is hereby granted, free of charge, to any person obtaining a copy of this software and associated documentation files (the "Software"), to deal in the Software without restriction, including without limitation the rights to use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of the Software, and to permit persons to whom the Software is furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
