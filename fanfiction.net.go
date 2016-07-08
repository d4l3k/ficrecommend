package main

import (
	"bytes"
	"log"
	"math/rand"
	"net/http"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/valyala/fasthttp"
)

func init() {
	scrapers = append(scrapers, scrapeFFnet)
	recommenders = append(recommenders, recommendFFnet)
}

var ffnetRegex = regexp.MustCompile(`^https?:\/\/.*fanfiction\.net\/s\/(\d+).*$`)

func recommendFFnet(s *server, url string, limit, offset int) (recResp, error) {
	if matches := ffnetRegex.FindStringSubmatch(url); len(matches) == 2 {
		st := Story{
			Id:   atoi(matches[1]),
			Site: Site_FFNET,
		}
		return recommendationStory(s, st, limit, offset)
	}
	return recResp{}, errStoryNotFound
}

func scrapeFFnet(s *server) {
	log.Println("Scraping fanfiction.net...")
	total := 8043930
	fetched := 1
	jobs := make(chan *User)

	type job struct {
		u   *User
		doc *goquery.Document
	}
	docs := make(chan job)

	// Launch goroutines to fetch documents
	client := &fasthttp.Client{}
	for j := 0; j < 100; j++ {
		go func() {
			var buf []byte
			for u := range jobs {
				url := "https://www.fanfiction.net/u/" + u.Id
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

	// Creates jobs
	go func() {
		for {
			u := &User{
				Id:   itoa(int32(rand.Intn(total))),
				Site: Site_FFNET,
			}
			if u.checkExists(s.graph) {
				continue
			}
			//time.Sleep(time.Second)
			jobs <- u
		}
	}()

	// Handle fetched documents
	for doc := range docs {
		u := doc.u
		err := u.fetch(doc.doc, s)
		if err != nil {
			log.Println(err)
		}
		fetched++
		if u.Exists {
			log.Printf("Fetched FF.net %8s %q", u.Id, u.Name)
		}
	}
}

var ffFavCountRegex = regexp.MustCompile(`Favs: ([0-9,]+) -`)

func (u *User) fetch(doc *goquery.Document, sr *server) error {
	if doc.Find("#bio_text").Length() != 1 {
		return u.save(sr)
	}
	u.Exists = true
	u.Name = strings.TrimSpace(doc.Find("#content_wrapper_inner span").First().Text())
	var err error
	st := Story{
		Site:   Site_FFNET,
		Exists: true,
	}
	for _, typ := range []string{".favstories", ".mystories"} {
		doc.Find(typ).Each(func(i int, s *goquery.Selection) {
			st.Id = atoi(s.AttrOr("data-storyid", ""))
			st.Category = s.AttrOr("data-category", "")
			st.Title = s.AttrOr("data-title", "")
			st.WordCount = atoi(s.AttrOr("data-wordcount", ""))
			st.DateSubmit = atoi(s.AttrOr("data-datesubmit", ""))
			st.DateUpdate = atoi(s.AttrOr("data-dateupdate", ""))
			st.Reviews = atoi(s.AttrOr("data-ratingtimes", ""))
			st.Chapters = atoi(s.AttrOr("data-chapters", ""))
			st.Complete = s.AttrOr("data-statusid", "") == "2"
			st.Image = s.Find("img").AttrOr("data-original", "")

			contentDiv := s.Find("div").First()
			html, _ := contentDiv.Html()
			st.Desc = html
			meta := contentDiv.Find("div").Text()
			matches := ffFavCountRegex.FindStringSubmatch(meta)
			if len(matches) == 2 {
				st.Favorites = atoi(strings.Replace(matches[1], ",", "", -1))
			}

			if !st.checkExistsTitle(sr.graph) {
				err = st.save(sr)
				if err != nil {
					return
				}
			}
			switch typ {
			case ".favstories":
				u.FavStories = append(u.FavStories, string(st.key()))
			case ".mystories":
				u.Stories = append(u.Stories, string(st.key()))
			}
		})
		if err != nil {
			return err
		}
	}
	doc.Find("#fa a").Each(func(i int, s *goquery.Selection) {
		link := s.AttrOr("href", "/u/0/a")
		auth := strings.Split(link, "/")[2]
		u.FavAuthors = append(u.FavAuthors, auth)
	})
	return u.save(sr)
}
