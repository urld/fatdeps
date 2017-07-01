# fatdeps

[![GoDoc](https://godoc.org/github.com/urld/fatdeps?status.svg)](https://godoc.org/github.com/urld/fatdeps)

`fatdeps` generates package dependency graphs for Go packages, and shows the size of each package.

This is inspired this article: [A Story of a Fat Go Binary](https://hackernoon.com/a-story-of-a-fat-go-binary-20edc6549b97)

The implementation is based on [github.com/davecheney/graphpkg](https://github.com/davecheney/graphpkg/).


## Installation

First install [graphviz](http://graphviz.org/Download.php) for your OS, then

	go get github.com/urld/fatdeps

## Usage

```fatdeps github.com/urld/helloworld```

![Example](https://github.com/urld/fatdeps/raw/master/example.png)


## TODO

* http server for interactive graph navigation
* show cumulative sizes
