package main

import (
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/boltdb/bolt"
)

func init() {
	scrapers = append(scrapers, scrapeFFnet)
	recommenders = append(recommenders, recommendFFnet)
}

var ffnetRegex = regexp.MustCompile(`^https?:\/\/.*fanfiction\.net\/s\/(\d+).*$`)

func recommendFFnet(db *bolt.DB, url string, limit, offset int) (*recResp, error) {
	if matches := ffnetRegex.FindStringSubmatch(url); len(matches) == 2 {
		s := &Story{
			Id:   atoi(matches[1]),
			Site: Site_FFNET,
		}
		return recommendationStory(db, s, limit, offset)

	}
	return nil, errStoryNotFound
}

func scrapeFFnet(db *bolt.DB) {
	log.Println("Scraping fanfiction.net...")
	db.Update(func(tx *bolt.Tx) error {
		for _, bucket := range []string{"stories", "users"} {
			_, err := tx.CreateBucketIfNotExists([]byte(bucket))
			if err != nil {
				return fmt.Errorf("create bucket: %s", err)
			}
		}
		return nil
	})
	total := 6832538
	fetched := 1
	jobs := make(chan *User)

	type job struct {
		u   *User
		doc *goquery.Document
	}
	docs := make(chan job)

	// 10 goroutines to fetch documents
	for j := 0; j < 10; j++ {
		go func() {
			for u := range jobs {
				client := &http.Client{}
				req, err := http.NewRequest("GET", "https://www.fanfiction.net/u/"+u.Id, nil)
				if err != nil {
					log.Println(err)
					continue
				}
				req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/42.0.2311.90 Safari/537.36")
				resp, err := client.Do(req)
				if err != nil {
					log.Println(err)
					continue
				}
				doc, err := goquery.NewDocumentFromResponse(resp)
				if err != nil {
					log.Println(err)
					continue
				}
				docs <- job{u, doc}
			}
		}()
	}

	// Creates jobs
	go func() {
		for {
			u := &User{
				Id:   itoa(int32(rand.Intn(total))),
				Site: Site_FFNET,
			}
			if u.update(db) {
				continue
			}
			time.Sleep(time.Second)
			jobs <- u
		}
	}()

	// Handle fetched documents
	for doc := range docs {
		u := doc.u
		err := u.fetch(doc.doc, db)
		if err != nil {
			log.Println(err)
		}
		fetched++
		if u.Exists {
			log.Println("Fetched FF.net", u.Name, u.Id)
		}
	}
}

func (u *User) fetch(doc *goquery.Document, db *bolt.DB) error {
	if doc.Find("#bio_text").Length() != 1 {
		return u.save(db)
	}
	u.Exists = true
	u.Name = strings.TrimSpace(doc.Find("#content_wrapper_inner span").First().Text())
	for _, typ := range []string{".favstories", ".mystories"} {
		doc.Find(typ).Each(func(i int, s *goquery.Selection) {
			html, _ := s.Find("div").First().Html()
			st := &Story{
				Id:         atoi(s.AttrOr("data-storyid", "")),
				Site:       Site_FFNET,
				Category:   s.AttrOr("data-category", ""),
				Title:      s.AttrOr("data-title", ""),
				WordCount:  atoi(s.AttrOr("data-wordcount", "")),
				DateSubmit: atoi(s.AttrOr("data-datesubmit", "")),
				DateUpdate: atoi(s.AttrOr("data-dateupdate", "")),
				Reviews:    atoi(s.AttrOr("data-ratingtimes", "")),
				Chapters:   atoi(s.AttrOr("data-chapters", "")),
				Complete:   s.AttrOr("data-statusid", "") == "2",
				Image:      s.Find("img").AttrOr("data-original", ""),
				Desc:       html,
			}
			st.update(db)
			st.FavedBy = append(st.FavedBy, string(u.key()))
			st.save(db)
			switch typ {
			case ".favstories":
				u.FavStories = append(u.FavStories, string(st.key()))
			case ".mystories":
				u.Stories = append(u.Stories, string(st.key()))
			}
		})
	}
	doc.Find("#fa a").Each(func(i int, s *goquery.Selection) {
		link := s.AttrOr("href", "/u/0/a")
		auth := strings.Split(link, "/")[2]
		u.FavAuthors = append(u.FavAuthors, auth)
	})
	return u.save(db)
}
