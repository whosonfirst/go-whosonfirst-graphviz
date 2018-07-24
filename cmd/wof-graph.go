package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"github.com/awalterschulze/gographviz"
	"github.com/tidwall/gjson"
	"github.com/whosonfirst/go-whosonfirst-cli/flags"
	"github.com/whosonfirst/go-whosonfirst-geojson-v2"
	"github.com/whosonfirst/go-whosonfirst-geojson-v2/feature"
	"github.com/whosonfirst/go-whosonfirst-geojson-v2/properties/whosonfirst"
	"github.com/whosonfirst/go-whosonfirst-index"
	"github.com/whosonfirst/go-whosonfirst-index/utils"
	fs_reader "github.com/whosonfirst/go-whosonfirst-readwrite-fs/reader"
	"github.com/whosonfirst/go-whosonfirst-readwrite/reader"
	"github.com/whosonfirst/go-whosonfirst-uri"
	"github.com/whosonfirst/warning"
	"io"
	"io/ioutil"
	"log"
	"path/filepath"
	"strconv"
	"sync"
)

func label(f geojson.Feature) string {

	name := whosonfirst.Name(f)
	wofid := whosonfirst.Id(f)

	str_label := fmt.Sprintf("\"%s (%d)\"", name, wofid)

	rsp := gjson.GetBytes(f.Bytes(), "properties.wof:label")

	if rsp.Exists() {

		 str_label = fmt.Sprintf("\"%s (%d)\"", rsp.String(), wofid)
	}

	return str_label
}

func parent(r reader.Reader, id int64) (geojson.Feature, error) {

	rel_path, err := uri.Id2RelPath(id)

	if err != nil {
		return nil, err
	}

	fh, err := r.Read(rel_path)

	if err != nil {
		return nil, err
	}

	f, err := feature.LoadWOFFeatureFromReader(fh)

	if err != nil {
		return nil, err
	}

	return f, nil
}

func main() {

	var to_exclude flags.MultiString
	flag.Var(&to_exclude, "exclude", "One or more placetypes to exclude")

	var mode = flag.String("mode", "repo", "")

	flag.Parse()

	graph := gographviz.NewGraph()

	err := graph.SetName("G")

	if err != nil {
		log.Fatal(err)
	}

	err = graph.SetDir(true)

	if err != nil {
		log.Fatal(err)
	}
	
	mu := new(sync.RWMutex)

	var r reader.Reader

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

		placetype := whosonfirst.Placetype(f)

		if to_exclude.Contains(placetype) {
			return nil
		}

		mu.Lock()
		defer mu.Unlock()

		parent_id := whosonfirst.ParentId(f)

		f_label := label(f)
		p_label := strconv.FormatInt(parent_id, 10)

		p, err := parent(r, parent_id)

		if err != nil {
			log.Printf("failed to load record for %d, %s\n", parent_id, err)
		} else {
			p_label = label(p)

			/*
				p_placetype := whosonfirst.Placetype(p)
				graph.AddNode("G", p_placetype, nil)
				graph.AddEdge(p_label, p_placetype, true, nil)
			*/
		}

		graph.AddNode("G", f_label, nil)
		graph.AddNode("G", p_label, nil)
		graph.AddEdge(f_label, p_label, true, nil)

		/*
			graph.AddNode("G", placetype, nil)
			graph.AddEdge(f_label, placetype, true, nil)
		*/

		return nil
	}

	if *mode != "repo" {
		log.Fatal("Only -mode repo is supported right now, sorry.")
	}

	i, err := index.NewIndexer(*mode, cb)

	if err != nil {
		log.Fatal(err)
	}

	for _, path := range flag.Args() {

		data := filepath.Join(path, "data")

		rr, err := fs_reader.NewFSReader(data)

		if err != nil {
			log.Fatal(err)
		}

		r = rr

		err = i.IndexPath(path)

		if err != nil {
			log.Fatal(err)
		}
	}

	fmt.Println(graph.String())
}
