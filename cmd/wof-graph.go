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
	"strconv"
	"sync"
)

func label(f geojson.Feature) string {

	name := whosonfirst.Name(f)
	placetype := whosonfirst.Placetype(f)
	wofid := whosonfirst.Id(f)

	rsp := gjson.GetBytes(f.Bytes(), "properties.wof:label")

	if rsp.Exists() {
		name = rsp.String()
	}

	str_label := fmt.Sprintf("\"%s, %s / %d\"", name, placetype, wofid)
	return str_label
}

func load(r reader.Reader, id int64) (geojson.Feature, error) {

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

func apply_superseded_by(graph *gographviz.Graph, f geojson.Feature, r reader.Reader, recursive bool) {

	f_label := label(f)

	for _, other_id := range whosonfirst.SupersededBy(f) {

		other_label := strconv.FormatInt(other_id, 10)
		other, err := load(r, other_id)

		if err != nil {
			log.Printf("failed to load record for %d, %s\n", other_id, err)
			continue
		}

		other_label = label(other)

		graph.AddNode("G", other_label, nil)
		graph.AddEdge(f_label, other_label, true, nil)

		if recursive {
			apply_superseded_by(graph, other, r, recursive)
		}
	}

}

func apply_supersedes(graph *gographviz.Graph, f geojson.Feature, r reader.Reader, recursive bool) {

	f_label := label(f)

	for _, other_id := range whosonfirst.Supersedes(f) {

		other_label := strconv.FormatInt(other_id, 10)
		other, err := load(r, other_id)

		if err != nil {
			log.Printf("failed to load record for %d, %s\n", other_id, err)
			continue
		}

		other_label = label(other)

		graph.AddNode("G", other_label, nil)
		graph.AddEdge(other_label, f_label, true, nil)

		if recursive {
			apply_supersedes(graph, other, r, recursive)
		}
	}
}

func main() {

	var to_exclude flags.MultiString
	flag.Var(&to_exclude, "exclude", "One or more placetypes to exclude")

	var belongs_to flags.MultiInt64
	flag.Var(&belongs_to, "belongs-to", "One or more WOF ID that a record should belong to")

	var superseded_by = flag.Bool("superseded_by", false, "Include superseded_by relationships")
	var supersedes = flag.Bool("supersedes", false, "Include supersedes relationships")

	var recursive = flag.Bool("recursive", false, "Recursive include superseding or superseded_by relationships")

	// var reader = flag.String("reader", "fs", "...")
	var dsn = flag.String("reader-dsn", "", "...")

	var mode = flag.String("mode", "repo", "Currently only '-mode repo' is supported")

	flag.Parse()

	r, err := fs_reader.NewFSReader(*dsn)

	if err != nil {
		log.Fatal(err)
	}

	graph := gographviz.NewGraph()

	err = graph.SetName("G")

	if err != nil {
		log.Fatal(err)
	}

	err = graph.SetDir(true)

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

		d, err := whosonfirst.IsDeprecated(f)

		if err != nil {
			return err
		}

		if d.IsTrue() && d.IsKnown() {
			return nil
		}

		placetype := whosonfirst.Placetype(f)

		if to_exclude.Contains(placetype) {
			return nil
		}

		if len(belongs_to) > 0 {

			skip := true

			for _, id := range belongs_to {

				if whosonfirst.IsBelongsTo(f, id) {
					skip = false
					break
				}
			}

			if skip == true {
				return nil
			}
		}

		mu.Lock()
		defer mu.Unlock()

		f_label := label(f)
		graph.AddNode("G", f_label, nil)

		if *superseded_by {
			apply_superseded_by(graph, f, r, *recursive)
		}

		if *supersedes {
			apply_supersedes(graph, f, r, *recursive)
		}

		parent_id := whosonfirst.ParentId(f)
		p_label := strconv.FormatInt(parent_id, 10)

		p, err := load(r, parent_id)

		if err != nil {
			log.Printf("failed to load record for %d, %s\n", parent_id, err)
		} else {
			p_label = label(p)
		}

		graph.AddNode("G", p_label, nil)
		graph.AddEdge(f_label, p_label, true, nil)

		return nil
	}

	i, err := index.NewIndexer(*mode, cb)

	if err != nil {
		log.Fatal(err)
	}

	for _, path := range flag.Args() {

		err = i.IndexPath(path)

		if err != nil {
			log.Fatal(err)
		}
	}

	fmt.Println(graph.String())
}
