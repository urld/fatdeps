# fatdeps

[![Go Report Card](https://goreportcard.com/badge/github.com/urld/fatdeps)](https://goreportcard.com/report/github.com/urld/fatdeps)
[![GoDoc](https://godoc.org/github.com/urld/fatdeps?status.svg)](https://godoc.org/github.com/urld/fatdeps)

`fatdeps` generates package dependency graphs for Go packages, and shows the size of each package.

This is inspired this article: [A Story of a Fat Go Binary](https://hackernoon.com/a-story-of-a-fat-go-binary-20edc6549b97)

The implementation is based on [github.com/davecheney/graphpkg](https://github.com/davecheney/graphpkg/).

## Installation

First install [graphviz](http://graphviz.org/Download.php) for your OS, then

	go get github.com/urld/fatdeps

## Usage

Start a http server and open the graph in browser:

	fatdeps github.com/urld/helloworld

All the nodes are linked. You can also filter nodes with a query parameter:

	http://localhost:8080/github.com/urld/helloworld?match=runtime
	
To get more detailed size information you can set the parameter ```symsize=true```:

	http://localhost:8080/github.com/urld/helloworld?symsize=true

This only works for commands and requires the compiled binary to be present in ```$GOBIN```.

![Example](https://github.com/urld/fatdeps/raw/master/example.png)



## TODO

* improve filter
