# Contributing to Skipper

**Thank you for your interest in making Skipper even better and more awesome. Your contributions are highly welcome.**

There are multiple ways of getting involved:

- [Report a bug](#report-a-bug)
- [Suggest a feature](#suggest-a-feature)
- [Contribute code](#contribute-code)

Below are a few guidelines we would like you to follow.
If you need help, please reach out to us: [community channels](https://github.com/zalando/skipper#community)


## Report a bug

Reporting bugs is one of the best ways to contribute. Before creating a bug report, please check that an [issue](https://github.com/zalando/skipper/issues) reporting the same problem does not already exist. If there is such an issue, you may add your information as a comment.

To report a new bug you should open an issue that summarizes the bug and set the label to "bug".

If you want to provide a fix along with your bug report: That is great! In this case please send us a pull request as described in section [Contribute Code](#contribute-code).

## Suggest a feature

To request a new feature you should open an [issue](https://github.com/zalando/skipper/issues/new) and summarize the desired functionality and its use case. Set the issue label to "enhancement".

## Contribute code

This is a rough outline of what the workflow for code contributions
looks like:

- Check the list of open [issues](https://github.com/zalando/skipper/issues). Either assign an existing issue to yourself, or create a new one that you would like work on and discuss your ideas and use cases.
- Fork the repository on GitHub
- Create a topic branch, for example feature/foo fix/bar refactor/baz, from where you want to base your work. The base is usually master.
- Make commits of logical units and use `git commit --sign-off` to comply with [DCO](https://developercertificate.org/).
- Write good commit messages (see below).
- Push your changes to a topic branch in your fork of the repository.
- Submit a pull request to [zalando/skipper](https://github.com/zalando/skipper)
- Your pull request must receive a :thumbsup: from two [Maintainers](https://github.com/zalando/skipper/blob/master/MAINTAINERS)
- Major changes need to include tests. Features need
  additionally include documentation for developers as
  [godoc](https://pkg.go.dev/github.com/zalando/skipper) and add
  [user documentation in markdown](https://opensource.zalando.com/skipper) in the docs/ directory.

Thanks for your contributions!

### Code style

Skipper is formatted with [gofmt](https://golang.org/cmd/gofmt/). Please run it on your code before making a pull request. The coding style suggested by the Golang community is the preferred one for the cases that are not covered by gofmt, see the [style doc](https://github.com/golang/go/wiki/CodeReviewComments) for details.

### Commit messages

Your commit messages ideally can answer two questions: what changed and why. The subject line should feature the “what” and the body of the commit should describe the “why”.

When creating a pull request, its comment should reference the corresponding issue id.

**Have fun and enjoy hacking!**

## Governance - final decisions

The project owner and lead makes all final decisions, if there is a
disagreement between contributors and maintainers.
