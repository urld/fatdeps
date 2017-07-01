# fatdeps

[![GoDoc](https://godoc.org/github.com/urld/fatdeps?status.svg)](https://godoc.org/github.com/urld/fatdeps)

`fatdeps` generates package dependency graphs for Go packages, and shows the size of each package.

This is inspired this article: [A Story of a Fat Go Binary](https://hackernoon.com/a-story-of-a-fat-go-binary-20edc6549b97)

The implementation is based on [github.com/davecheney/graphpkg](https://github.com/davecheney/graphpkg/).

Note that the actual binary will usually have a much smaller size, since only the intermediate '.a' files
of the packages are analyzed.

## Installation

First install [graphviz](http://graphviz.org/Download.php) for your OS, then

	go get github.com/urld/fatdeps

## Usage

Start a http server and open the graph in browser:

	fatdeps github.com/urld/helloworld

All the nodes are linked. You can also filter nodes with a query parameter:

	http://localhost:8080/github.com/urld/helloworld?match=runtime

![Example](https://github.com/urld/fatdeps/raw/master/example.png)



## TODO

* show cumulative sizes
* improve filter
