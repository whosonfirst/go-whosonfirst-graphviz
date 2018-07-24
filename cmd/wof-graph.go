package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"github.com/awalterschulze/gographviz"
	"github.com/whosonfirst/go-whosonfirst-geojson-v2/feature"
	"github.com/whosonfirst/go-whosonfirst-geojson-v2/properties/whosonfirst"
	"github.com/whosonfirst/go-whosonfirst-index"
	"github.com/whosonfirst/go-whosonfirst-index/utils"
	"github.com/whosonfirst/warning"
	"io"
	"io/ioutil"
	"log"
	"strconv"
	"sync"
)

func main() {

	var mode = flag.String("mode", "repo", "")

	flag.Parse()

	graph := gographviz.NewGraph()

	ast, _ := gographviz.ParseString(`digraph G {}`)
	err := gographviz.Analyse(ast, graph)

	if err != nil {
		log.Fatal(err)
	}

	mu := new(sync.RWMutex)

	cb := func(fh io.Reader, ctx context.Context, args ...interface{}) error {

		path, err := index.PathForContext(ctx)

		if err != nil {
			return err
		}

		ok, err := utils.IsPrincipalWOFRecord(fh, ctx)

		if err != nil {
			return err
		}

		if !ok {
			return nil
		}

		closer := ioutil.NopCloser(fh)

		f, err := feature.LoadWOFFeatureFromReader(closer)

		if err != nil && !warning.IsWarning(err) {
			msg := fmt.Sprintf("Unable to load %s, because %s", path, err)
			return errors.New(msg)
		}

		name := whosonfirst.Name(f)
		wofid := whosonfirst.Id(f)
		parent_id := whosonfirst.ParentId(f)

		str_wofid := strconv.FormatInt(wofid, 10)
		str_parentid := strconv.FormatInt(parent_id, 10)

		placetype := whosonfirst.Placetype(f)

		whoami := fmt.Sprintf("\"%s (%d)\"", name, wofid)
		
		mu.Lock()
		defer mu.Unlock()

		graph.AddNode("G", str_wofid, nil)
		graph.AddNode("G", whoami, nil)
		graph.AddNode("G", str_parentid, nil)
		graph.AddNode("G", placetype, nil)

		graph.AddEdge(whoami, str_parentid, true, nil)
		graph.AddEdge(whoami, str_wofid, true, nil)

		return nil
	}

	i, err := index.NewIndexer(*mode, cb)

	if err != nil {
		log.Fatal(err)
	}

	for _, path := range flag.Args() {

		err := i.IndexPath(path)

		if err != nil {
			log.Fatal(err)
		}
	}

	fmt.Println(graph.String())
}
