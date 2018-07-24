# go-whosonfirst-graphviz

## Install

You will need to have both `Go` (specifically a version of Go more recent than 1.7 so let's just assume you need [Go 1.9](https://golang.org/dl/) or higher) and the `make` programs installed on your computer. Assuming you do just type:

```
make bin
```

All of this package's dependencies are bundled with the code in the `vendor` directory.

## Tools

### wof-graph

```
./bin/wof-graph -h
Usage of ./bin/wof-graph:
  -belongs-to value
    	One or more WOF ID that a record should belong to
  -exclude value
    	One or more placetypes to exclude
  -mode string
    	Currently only '-mode repo' is supported (default "repo")
  -superseded_by
    	Include superseded_by relationships
  -supersedes
    	Include supersedes relationships
```	

## See also

* https://github.com/awalterschulze/gographviz
* https://www.graphviz.org/doc/info/command.html
* https://graphs.grevian.org/example