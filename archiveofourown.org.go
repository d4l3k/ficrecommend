package main

import (
	"errors"
	"log"
	"net/http"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

func init() {
	scrapers = append(scrapers, scrapeAO3)
	recommenders = append(recommenders, recommendAO3)
}

var ao3Regex = regexp.MustCompile(`^https?:\/\/archiveofourown.org\/works\/(\d+).*$`)

func recommendAO3(sr *server, url string, limit, offset int) (*recResp, error) {
	if matches := ao3Regex.FindStringSubmatch(url); len(matches) == 2 {
		s := &Story{
			Id:   atoi(matches[1]),
			Site: Site_AO3,
		}
		return recommendationStory(sr, s, limit, offset)
	}
	return nil, errStoryNotFound
}

func scrapeAO3(sr *server) {
	log.Println("Scraping archiveofourown.org...")
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
			if u.checkExists(sr.graph) {
				continue
			}
			//time.Sleep(time.Second / 2)
			jobs <- u
		}
	}()

	// Handle fetched documents
	for doc := range docs {
		s := doc.s
		err := fetchAO3(s, doc.doc, sr)
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

func fetchAO3(s *Story, doc *goquery.Document, sr *server) error {
	if doc.Find("h2.title.heading").Length() == 0 {
		s.Exists = false
		if err := s.save(sr); err != nil {
			return err
		}
		return errors.New("story doesn't exist " + itoa(s.Id))
	}
	s.Exists = true
	s.Title = strings.TrimSpace(doc.Find("h2.title.heading").First().Text())
	s.Desc = doc.Find(".summary p").Text()
	stats, _ := doc.Find("dd.stats dl.stats").Html()
	fandoms, _ := doc.Find(".fandom li").Html()
	s.Desc += "<div class='xgray'>" + fandoms + " - " + stats + "</div>"

	var err error
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
		u.Exists = true
		s.FavedBy = append(s.FavedBy, u.key())
		u.FavStories = append(u.FavStories, s.key())
		if !u.checkExists(sr.graph) {
			err = u.save(sr)
			if err != nil {
				return
			}
		}
	})
	if err != nil {
		return err
	}
	return s.save(sr)
}
