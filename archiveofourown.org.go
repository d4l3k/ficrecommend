package main

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/boltdb/bolt"
)

func init() {
	scrapers = append(scrapers, scrapeAO3)
	recommenders = append(recommenders, recommendAO3)
}

var ao3Regex = regexp.MustCompile(`^https?:\/\/archiveofourown.org\/works\/(\d+).*$`)

func recommendAO3(db *bolt.DB, url string, limit int) *recResp {
	if matches := ao3Regex.FindStringSubmatch(url); len(matches) == 2 {
		s := &Story{
			Id:   atoi(matches[1]),
			Site: Site_AO3,
		}
		return recommendationStory(db, s, limit)
	}
	return nil
}

func scrapeAO3(db *bolt.DB) {
	log.Println("Scraping archiveofourown.org...")
	db.Update(func(tx *bolt.Tx) error {
		for _, bucket := range []string{"stories", "users"} {
			_, err := tx.CreateBucketIfNotExists([]byte(bucket))
			if err != nil {
				return fmt.Errorf("create bucket: %s", err)
			}
		}
		return nil
	})
	fetched := 1
	total := 4200000

	type job struct {
		s   *Story
		doc *goquery.Document
	}
	jobs := make(chan *Story)
	docs := make(chan job)

	// 100 goroutines to fetch documents
	for j := 0; j < 100; j++ {
		go func() {
			for u := range jobs {
				client := &http.Client{}
				req, err := http.NewRequest("GET", "https://archiveofourown.org/works/"+itoa(u.Id), nil)
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

	i := int32(1)
	bad := 0

	// Creates jobs
	go func() {
		for {
			if bad > 5000 {
				return
			}
			u := &Story{
				Id:   i,
				Site: Site_AO3,
			}
			i++
			fetched++
			if u.update(db) {
				continue
			}
			//time.Sleep(time.Second / 2)
			jobs <- u
		}
	}()

	// Handle fetched documents
	for doc := range docs {
		s := doc.s
		err := fetchAO3(s, doc.doc, db)
		if err != nil {
			if !strings.HasPrefix(err.Error(), "story doesn't exist") {
				log.Println(err)
			} else {
				bad++
			}
			continue
		}
		bad = 0
		log.Println("Fetched AO3 ", s.Title, s.Id, fetched, total)
	}
}

func fetchAO3(s *Story, doc *goquery.Document, db *bolt.DB) error {
	if doc.Find("h2.title.heading").Length() == 0 {
		s.Exists = false
		s.save(db)
		return errors.New("story doesn't exist " + itoa(s.Id))
	}
	s.Title = strings.TrimSpace(doc.Find("h2.title.heading").First().Text())
	s.Desc = doc.Find(".summary p").Text()
	stats, _ := doc.Find("dd.stats dl.stats").Html()
	fandoms, _ := doc.Find(".fandom li").Html()
	s.Desc += "<div class='xgray'>" + fandoms + " - " + stats + "</div>"

	doc.Find("#kudos a").Each(func(i int, sel *goquery.Selection) {
		link := sel.AttrOr("href", "")
		if !strings.HasPrefix(link, "/users/") {
			return
		}

		name := strings.ToLower(sel.Text())

		u := &User{
			Id:   name,
			Name: name,
			Site: Site_AO3,
		}
		u.update(db)
		s.FavedBy = append(s.FavedBy, string(u.key()))
		u.FavStories = append(u.FavStories, string(s.key()))
		u.save(db)
	})
	return s.save(db)
}
