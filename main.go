package main

//go:generate protoc --gogoslick_out=. main.proto

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"

	_ "net/http/pprof"

	"github.com/dgraph-io/badger"
	"github.com/pkg/errors"
)

// FakeUserAgent is the user agent to use when making requests.
const FakeUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_11_5) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/51.0.2704.103 Safari/537.36"

func (s *Story) annotate() {
	switch s.Site {
	case FFNET:
		s.Url = "https://www.fanfiction.net/s/" + itoa(s.Id) + "/" + strings.Replace(s.Title, " ", "-", -1)
		s.Dl = "http://ficsave.xyz/?format=epub&e=&download=yes&url=" + s.Url
	case FICTIONPRESS:
		s.Url = "https://www.fictionpress.com/s/" + itoa(s.Id) + "/" + strings.Replace(s.Title, " ", "-", -1)
		s.Dl = "http://ficsave.xyz/?format=epub&e=&download=yes&url=" + s.Url
	case AO3:
		s.Url = "https://archiveofourown.org/works/" + itoa(s.Id)
		s.Dl = "https://archiveofourown.org/downloads/a/a/" + itoa(s.Id) + "/a.epub"
	}
}

func atoi(s string) int32 {
	i, _ := strconv.Atoi(s)
	return int32(i)
}
func itoa(i int32) string {
	return strconv.Itoa(int(i))
}

func (s *server) cmdGet(bucket, key string) error {
	var v interface{}
	var err error
	switch bucket {
	case "stories":
		v, err = s.storyByKey(key)
		if err != nil {
			return err
		}
	default:
		return errors.Errorf("unknown bucket: %q", bucket)
	}
	fmt.Printf("key=%s, value=%+v\n", key, v)
	return nil
}

type storySlice struct {
	arr []string
	m   map[string]float64
}

func (a *storySlice) Len() int           { return len(a.arr) }
func (a *storySlice) Swap(i, j int)      { a.arr[i], a.arr[j] = a.arr[j], a.arr[i] }
func (a *storySlice) Less(i, j int) bool { return a.m[a.arr[i]] > a.m[a.arr[j]] }

// sortMap returns the items in a map.
func sortMap(m map[string]float64) []string {
	ss := &storySlice{
		m:   m,
		arr: make([]string, 0, len(m)),
	}
	for k := range m {
		ss.arr = append(ss.arr, k)
	}
	sort.Sort(ss)
	return ss.arr
}

var recommenders []func(s *server, urls []string, limit, offset int) (recResp, error)

func (s *server) recommendations(url string, limit, offset int) (recResp, error) {
	urls := strings.Split(url, "|")
	for _, rec := range recommenders {
		resp, err := rec(s, urls, limit, offset)
		if err == errStoryNotFound {
			continue
		} else if err != nil {
			return recResp{}, err
		}
		return resp, nil
	}
	return recResp{}, errStoryNotFound
}

var scrapers []func(s *server)

func cmdRecommend(s *server, id string) {
	recs, err := s.recommendations(id, 20, 0)
	if err != nil {
		log.Print(err)
		return
	}
	log.Printf("Recommended stories:")
	for _, st := range recs.Stories {
		log.Printf("  %s (%v)", st.Title, st.Id)
	}
	log.Printf("Recommended authors:")
	for _, st := range recs.Authors {
		log.Printf("  %s (%v)", st.Name, st.Id)
	}
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "static/index.html")
}

func requestFormInt(r *http.Request, field string, def int) int {
	val := r.FormValue(field)
	if len(val) == 0 {
		return def
	}
	num, err := strconv.Atoi(val)
	if err != nil {
		return def
	}
	return num
}

func (s *server) handleRecommendation(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")

	id := r.FormValue("id")
	limit := requestFormInt(r, "limit", 100)
	offset := requestFormInt(r, "offset", 0)
	if limit > 200 || limit < 0 {
		http.Error(w, "limit must be  <= 200 && >= 0", 400)
		return
	}
	if offset < 0 {
		http.Error(w, "offset must be  >= 0", 400)
		return
	}
	resp, err := s.recommendations(id, limit, offset)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")

	jsonBytes, err := json.Marshal(resp)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	callback := r.FormValue("callback")
	if callback != "" {
		fmt.Fprintf(w, "%s(%s)", callback, jsonBytes)
	} else {
		w.Write(jsonBytes)
	}
}

var (
	scrape = flag.Bool("scrape", true, "whether to scrape sites")
	port   = flag.String("port", "6060", "port to run on")
	dbpath = flag.String("dbpath", "./recommender.badger", "database directory")
)

func loadDB(path string) (*badger.DB, error) {
	opts := badger.DefaultOptions
	opts.Dir = path
	opts.ValueDir = path
	return badger.Open(opts)
}

type server struct {
	db *badger.DB
}

func newServer() (*server, error) {
	s := &server{}

	db, err := loadDB(*dbpath)
	if err != nil {
		return nil, err
	}
	s.db = db

	return s, nil
}

func (s *server) startScraping() {
	log.Print("starting scraping...")
	for _, scraper := range scrapers {
		go scraper(s)
	}
}

// Setup registers all server handlers.
func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	log.SetFlags(log.Flags() | log.Lshortfile)
	flag.Parse()

	s, err := newServer()
	if err != nil {
		return err
	}
	defer s.db.Close()

	args := flag.Args()
	log.Println(args)
	if len(args) > 0 {
		return s.cmdGet(args[0], args[1])
	}

	if *scrape {
		s.startScraping()
	}

	fs := http.FileServer(http.Dir("."))
	http.Handle("/static/", fs)

	http.HandleFunc("/", handleIndex)
	http.HandleFunc("/api/v1/recommendation", s.handleRecommendation)

	log.Printf("Serving on :%s...", *port)

	return http.ListenAndServe("0.0.0.0:"+*port, nil)
}
