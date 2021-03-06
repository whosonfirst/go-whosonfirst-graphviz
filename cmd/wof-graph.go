package main

import (
	"context"
	"crypto/md5"
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
	"github.com/whosonfirst/go-whosonfirst-readwrite/reader"
	"github.com/whosonfirst/go-whosonfirst-uri"
	"github.com/whosonfirst/warning"
	"io"
	"io/ioutil"
	"log"
	_ "path/filepath"
	"strconv"
	"sync"
)

func hex_colour(s string) string {

	hash := fmt.Sprintf("%x", md5.Sum([]byte(s)))
	return fmt.Sprintf("#%s", hash[0:6])
}

func label(f geojson.Feature) (string, map[string]string) {

	name := whosonfirst.Name(f)
	placetype := whosonfirst.Placetype(f)
	wofid := whosonfirst.Id(f)

	rsp := gjson.GetBytes(f.Bytes(), "properties.wof:label")

	if rsp.Exists() {
		name = rsp.String()
	}

	inception := "uuuu"
	cessation := "uuuu"

	rsp = gjson.GetBytes(f.Bytes(), "properties.edtf:inception")

	if rsp.Exists() {
		inception = rsp.String()
	}

	rsp = gjson.GetBytes(f.Bytes(), "properties.edtf:cessation")

	if rsp.Exists() {
		cessation = rsp.String()
	}

	dates := fmt.Sprintf("%s - %s", inception, cessation)
	c := hex_colour(dates)

	pc := hex_colour(placetype)

	var shape string

	switch placetype {
	case "building":
		shape = "rectangle"
	default:
		shape = "rectangle"
	}

	attrs := map[string]string{
		"shape":     shape,
		"style":     "filled",
		"color":     fmt.Sprintf("\"%s\"", pc),
		"fillcolor": fmt.Sprintf("\"%s\"", c),
		"fontcolor": "\"#ffffff\"",
	}

	str_label := fmt.Sprintf("\"%s, %s / %d\"", name, placetype, wofid)
	return str_label, attrs
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

	var sources flags.MultiString
	flag.Var(&sources, "source", "One or more filesystem based sources to use to read WOF ID data, which may or may not be part of the sources to graph. This is work in progress.")

	var to_exclude flags.MultiString
	flag.Var(&to_exclude, "exclude", "One or more placetypes to exclude")

	var belongs_to flags.MultiInt64
	flag.Var(&belongs_to, "belongs-to", "One or more WOF ID that a record should belong to")

	var superseded_by = flag.Bool("superseded_by", false, "Include superseded_by relationships")
	var supersedes = flag.Bool("supersedes", false, "Include supersedes relationships")

	var mode = flag.String("mode", "repo", "Currently only '-mode repo' is supported")

	flag.Parse()

	r, err := reader.NewMultiReaderFromStrings(sources...)

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

		/*
			if whosonfirst.IsCeased(f){
				return nil
			}
		*/

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

		f_label, f_attrs := label(f)
		graph.AddNode("G", f_label, f_attrs)

		if *superseded_by {

			for _, other_id := range whosonfirst.SupersededBy(f) {

				other_label := strconv.FormatInt(other_id, 10)
				other_attrs := make(map[string]string)

				other, err := parent(r, other_id)

				if err != nil {
					log.Printf("failed to load record for %d, %s\n", other_id, err)
				} else {

					other_label, other_attrs = label(other)

					graph.AddNode("G", other_label, other_attrs)
					graph.AddEdge(f_label, other_label, true, nil)
				}
			}
		}

		if *supersedes {

			for _, other_id := range whosonfirst.Supersedes(f) {

				other_label := strconv.FormatInt(other_id, 10)
				other_attrs := make(map[string]string)

				other, err := parent(r, other_id)

				if err != nil {
					log.Printf("failed to load record for %d, %s\n", other_id, err)
				} else {

					other_label, other_attrs = label(other)

					graph.AddNode("G", other_label, other_attrs)
					graph.AddEdge(other_label, f_label, true, nil)
				}
			}
		}

		parent_id := whosonfirst.ParentId(f)
		p_label := strconv.FormatInt(parent_id, 10)
		p_attrs := make(map[string]string)

		p, err := parent(r, parent_id)

		if err != nil {
			log.Printf("failed to load record for %d, %s\n", parent_id, err)
		} else {
			p_label, p_attrs = label(p)
		}

		graph.AddNode("G", p_label, p_attrs)
		graph.AddEdge(f_label, p_label, true, nil)

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

		err = i.IndexPath(path)

		if err != nil {
			log.Fatal(err)
		}
	}

	fmt.Println(graph.String())
}
