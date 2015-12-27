package main

//go:generate protoc --go_out=. main.proto

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime"
	"sort"
	"strconv"

	"github.com/boltdb/bolt"
)

func (s *Story) annotate() {
	switch s.Site {
	case Site_FFNET:
		s.Url = "https://www.fanfiction.net/s/" + itoa(s.Id) + "/" + s.Title
		s.Dl = "http://ficsave.com/?format=epub&e=&auto_download=yes&story_url=" + s.Url
	case Site_AO3:
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

func cmdList(db *bolt.DB, bucket string) {
	db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		b.ForEach(func(k, v []byte) error {
			fmt.Printf("key=%s, value=%s\n", k, v)
			return nil
		})
		return nil
	})
}

func cmdCount(db *bolt.DB, bucket string) {
	log.Println("Counting", bucket)
	db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		log.Println("Entries:", b.Stats().KeyN)
		return nil
	})
}

func cmdGet(db *bolt.DB, bucket, key string) {
	db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		var v interface{}
		switch bucket {
		case "stories":
			s := storyByKey(key)
			s.update(db)
			v = s
		case "users":
			u := userByKey(key)
			u.update(db)
			v = u
		default:
			v = b.Get([]byte(key))
		}
		fmt.Printf("key=%s, value=%+v\n", key, v)
		return nil
	})
}

type storySlice struct {
	arr []string
	m   map[string]int
}

func (a *storySlice) Len() int           { return len(a.arr) }
func (a *storySlice) Swap(i, j int)      { a.arr[i], a.arr[j] = a.arr[j], a.arr[i] }
func (a *storySlice) Less(i, j int) bool { return a.m[a.arr[i]] > a.m[a.arr[j]] }

// sortMap returns the top limit items in a map.
func sortMap(m map[string]int, limit int) []string {
	ss := &storySlice{
		m: m,
	}
	for k, priority := range m {
		if len(ss.arr) > limit && priority <= 1 {
			continue
		}
		ss.arr = append(ss.arr, k)
	}
	sort.Sort(ss)
	return ss.arr
}

func fetchStories(b *bolt.Bucket, keys []string) []*Story {
	stories := make([]*Story, len(keys))
	for i, key := range keys {
		st := storyByKey(key)
		st.updateBucket(b)
		stories[i] = st
	}
	return stories
}

func fetchUsers(b *bolt.Bucket, keys []string) []*User {
	users := make([]*User, len(keys))
	for i, key := range keys {
		st := userByKey(key)
		st.updateBucket(b)
		users[i] = st
	}
	return users
}

var recommenders []func(db *bolt.DB, url string, limit, offset int) (*recResp, error)

func recommendations(db *bolt.DB, url string, limit, offset int) (*recResp, error) {
	for _, rec := range recommenders {
		resp, err := rec(db, url, limit, offset)
		if err == errStoryNotFound {
			continue
		} else if err != nil {
			return nil, err
		}
		return resp, nil
	}
	return nil, errStoryNotFound
}

var scrapers []func(db *bolt.DB)

func cmdScrape(db *bolt.DB) {
	for _, scraper := range scrapers {
		go scraper(db)
	}
}

func cmdRecommend(db *bolt.DB, id string) {
	recs, err := recommendations(db, id, 20, 0)
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

var bdb *bolt.DB

type recResp struct {
	Stories []*Story
	Authors []*User
	Story   *Story
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

func handleRecommendation(w http.ResponseWriter, r *http.Request) {
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
	resp, err := recommendations(bdb, id, limit, offset)
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
)

// Setup registers all server handlers.
func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	flag.Parse()

	db, err := bolt.Open("recommender.db", 0600, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	bdb = db

	args := os.Args[1:]
	switch len(args) {
	case 2:
		switch args[0] {
		case "list":
			cmdList(db, args[1])
		case "recommend":
			cmdRecommend(db, args[1])
		case "count":
			cmdCount(db, args[1])
		}
		return
	case 3:
		if args[0] == "get" {
			cmdGet(db, args[1], args[2])
			return
		}
	}

	if *scrape {
		go cmdScrape(db)
	}

	fs := http.FileServer(http.Dir("."))
	http.Handle("/static/", fs)

	http.HandleFunc("/", handleIndex)
	http.HandleFunc("/api/v1/recommendation", handleRecommendation)

	log.Printf("Serving on :%s...", *port)

	err = http.ListenAndServe("0.0.0.0:"+*port, nil)
	if err != nil {
		log.Println(err)
	}
	log.Println("Server on 6060 stopped")

	os.Exit(0)

}
