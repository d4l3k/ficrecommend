package main

import (
	"bytes"
	"errors"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/valyala/fasthttp"
)

func init() {
	scrapers = append(scrapers, scrapeAO3)
	recommenders = append(recommenders, recommendAO3)
}

var ao3Regex = regexp.MustCompile(`^https?:\/\/archiveofourown.org\/works\/(\d+).*$`)

func recommendAO3(sr *server, url string, limit, offset int) (recResp, error) {
	if matches := ao3Regex.FindStringSubmatch(url); len(matches) == 2 {
		s := Story{
			Id:   atoi(matches[1]),
			Site: Site_AO3,
		}
		return recommendationStory(sr, s, limit, offset)
	}
	return recResp{}, errStoryNotFound
}

func getLatestAO3() (int, error) {
	doc, err := goquery.NewDocument("https://archiveofourown.org/works")
	if err != nil {
		return 0, err
	}
	bestTotal := 0
	doc.Find(".work .heading a").Each(func(i int, s *goquery.Selection) {
		bits := strings.Split(s.AttrOr("href", ""), "/")
		total, _ := strconv.Atoi(bits[len(bits)-1])
		if total > bestTotal {
			bestTotal = total
		}
	})
	if bestTotal == 0 {
		return 0, errors.New("failed to determine AO3 latest work")
	}
	return bestTotal, nil
}

func scrapeAO3(sr *server) {
	log.Println("Scraping archiveofourown.org...")
	fetched := 1
	total := 0

	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		for {
			newTotal, err := getLatestAO3()
			if err != nil {
				log.Print(err)
				time.Sleep(time.Second)
				continue
			}
			if newTotal > total {
				total = newTotal
			}
			log.Printf("Latest AO3 work %d", total)
			// Wait for ticker.
			<-ticker.C
		}
	}()

	type job struct {
		s   *Story
		doc *goquery.Document
	}
	jobs := make(chan *Story)
	docs := make(chan job)

	// Launch goroutines to fetch documents
	client := &fasthttp.Client{}
	for j := 0; j < 100; j++ {
		go func() {
			var buf []byte
			for u := range jobs {
				url := "http://archiveofourown.org/works/" + itoa(u.Id)
				statusCode, body, err := client.Get(buf, url)
				if err != nil {
					log.Println(err)
					continue
				}
				if statusCode != http.StatusOK && statusCode != http.StatusNotFound {
					log.Printf("fetch %q status code = %d", url, statusCode)
					continue
				}
				doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
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
			if int(i) > total {
				time.Sleep(time.Second)
				continue
			}
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
		log.Printf("Fetched AO3 %8d %q %d %d", s.Id, s.Title, fetched, total)
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
	u := User{
		Site:   Site_AO3,
		Exists: true,
	}
	doc.Find("#kudos a").Each(func(i int, sel *goquery.Selection) {
		link := sel.AttrOr("href", "")
		if !strings.HasPrefix(link, "/users/") {
			return
		}

		s.Favorites++

		name := strings.ToLower(sel.Text())
		u.Id = name
		u.Name = name
		u.FavStories = []string{s.key()}
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
