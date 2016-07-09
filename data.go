package main

import (
	"errors"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"time"

	"github.com/cayleygraph/cayley"
	"github.com/cayleygraph/cayley/graph"
	"github.com/cayleygraph/cayley/graph/path"
	"github.com/cayleygraph/cayley/quad"
)

var errStoryNotFound = errors.New("story not found in database")

// Site type properties.
const (
	SiteID = "/site/id"
)

// Types
const (
	typePrefix = "/type/"
	TypeType   = typePrefix + "type"
	TypeUser   = typePrefix + "user"
	TypeStory  = typePrefix + "story"
)

// User type properties.
const (
	userPrefix         = "/user/"
	UserID             = userPrefix + "id"
	UserName           = userPrefix + "name"
	UserStory          = userPrefix + "story"
	UserFavoriteStory  = userPrefix + "favorite_story"
	UserFavoriteAuthor = userPrefix + "favorite_author"
	UserFavoritedBy    = userPrefix + "favorited_by"
)

// Story type properties.
const (
	storyPrefix      = "/story/"
	StoryID          = storyPrefix + "id"
	StoryTitle       = storyPrefix + "title"
	StoryAuthor      = storyPrefix + "author"
	StoryCategory    = storyPrefix + "category"
	StoryImage       = storyPrefix + "image"
	StoryDescription = storyPrefix + "description"
	StoryURL         = storyPrefix + "url"
	StoryDL          = storyPrefix + "dl"
	StoryWordCount   = storyPrefix + "word_count"
	StoryDateSubmit  = storyPrefix + "date_submit"
	StoryDateUpdate  = storyPrefix + "date_update"
	StoryReviewCount = storyPrefix + "review_count"
	StoryFavorites   = storyPrefix + "favorites"
	StoryChapters    = storyPrefix + "chapters"
	StoryComplete    = storyPrefix + "complete"
	StoryFavoritedBy = storyPrefix + "favorited_by"
)

func (u User) checkExists(g *cayley.Handle) bool {
	it, _ := cayley.StartPath(g, u.key()).Out(UserID).BuildIterator().Optimize()
	defer it.Close()
	return cayley.RawNext(it)
}

func (s Story) checkExists(g *cayley.Handle) bool {
	it, _ := cayley.StartPath(g, s.key()).Out(StoryID).BuildIterator().Optimize()
	defer it.Close()
	return cayley.RawNext(it)
}

func (s Story) checkExistsTitle(g *cayley.Handle) bool {
	it, _ := cayley.StartPath(g, s.key()).Out(StoryTitle).BuildIterator().Optimize()
	defer it.Close()
	return cayley.RawNext(it)
}

func (u User) key() string {
	return "user:" + Site_name[int32(u.Site)] + ":" + u.Id
}

func (u User) save(s *server) error {
	id := u.key()
	txn := graph.NewTransaction()
	txn.AddQuad(quad.Quad{Subject: id, Predicate: TypeType, Object: TypeUser})
	txn.AddQuad(quad.Quad{Subject: id, Predicate: UserID, Object: u.Id})
	txn.AddQuad(quad.Quad{Subject: id, Predicate: SiteID, Object: Site_name[int32(u.Site)]})
	if u.Exists {
		txn.AddQuad(quad.Quad{Subject: id, Predicate: UserName, Object: u.Name})
		for _, story := range u.Stories {
			txn.AddQuad(quad.Quad{Subject: id, Predicate: UserStory, Object: story})
			txn.AddQuad(quad.Quad{Subject: story, Predicate: StoryAuthor, Object: id})
		}
		for _, story := range u.FavStories {
			txn.AddQuad(quad.Quad{Subject: id, Predicate: UserFavoriteStory, Object: story})
			txn.AddQuad(quad.Quad{Subject: story, Predicate: StoryFavoritedBy, Object: id})
		}
		for _, author := range u.FavAuthors {
			txn.AddQuad(quad.Quad{Subject: id, Predicate: UserFavoriteAuthor, Object: author})
			txn.AddQuad(quad.Quad{Subject: author, Predicate: UserFavoritedBy, Object: id})
		}
	}
	return s.graph.ApplyTransaction(txn)
}

func storyByKey(g *cayley.Handle, key string) Story {
	stories := storiesByKeys(g, []string{key})
	if len(stories) > 0 {
		return *stories[0]
	}
	return Story{}
}

func storiesByKeys(g *cayley.Handle, keys []string) []*Story {
	stories := make([]*Story, 0, len(keys))
	it, _ := cayley.StartPath(g, keys...).Save(StoryID, StoryID).Save(StoryTitle, StoryTitle).Save(StoryDescription, StoryDescription).Save(SiteID, SiteID).BuildIterator().Optimize()
	defer it.Close()
	for cayley.RawNext(it) {
		s := Story{}
		results := map[string]graph.Value{}
		it.TagResults(results)
		s.Id = atoi(g.NameOf(results[StoryID]))
		s.Title = g.NameOf(results[StoryTitle])
		s.Desc = g.NameOf(results[StoryDescription])
		s.Site = Site(Site_value[g.NameOf(results[SiteID])])
		s.annotate()
		stories = append(stories, &s)
	}
	return stories
}

func (s Story) key() string {
	return "story:" + Site_name[int32(s.Site)] + ":" + itoa(s.Id)
}

func (s Story) save(sr *server) error {
	id := s.key()
	txn := graph.NewTransaction()
	txn.AddQuad(quad.Quad{Subject: id, Predicate: TypeType, Object: TypeStory})
	txn.AddQuad(quad.Quad{Subject: id, Predicate: StoryID, Object: itoa(s.Id)})
	txn.AddQuad(quad.Quad{Subject: id, Predicate: SiteID, Object: Site_name[int32(s.Site)]})
	if s.Exists {
		txn.AddQuad(quad.Quad{Subject: id, Predicate: StoryTitle, Object: s.Title})
		txn.AddQuad(quad.Quad{Subject: id, Predicate: StoryCategory, Object: s.Category})
		txn.AddQuad(quad.Quad{Subject: id, Predicate: StoryImage, Object: s.Image})
		txn.AddQuad(quad.Quad{Subject: id, Predicate: StoryDescription, Object: s.Desc})
		txn.AddQuad(quad.Quad{Subject: id, Predicate: StoryURL, Object: s.Url})
		txn.AddQuad(quad.Quad{Subject: id, Predicate: StoryDL, Object: s.Dl})
		txn.AddQuad(quad.Quad{Subject: id, Predicate: StoryWordCount, Object: itoa(s.WordCount)})
		txn.AddQuad(quad.Quad{Subject: id, Predicate: StoryDateSubmit, Object: itoa(s.DateSubmit)})
		txn.AddQuad(quad.Quad{Subject: id, Predicate: StoryDateUpdate, Object: itoa(s.DateUpdate)})
		txn.AddQuad(quad.Quad{Subject: id, Predicate: StoryReviewCount, Object: itoa(s.Reviews)})
		txn.AddQuad(quad.Quad{Subject: id, Predicate: StoryChapters, Object: itoa(s.Chapters)})
		txn.AddQuad(quad.Quad{Subject: id, Predicate: StoryFavorites, Object: itoa(s.Favorites)})
		txn.AddQuad(quad.Quad{Subject: id, Predicate: StoryComplete, Object: strconv.FormatBool(s.Complete)})
		for _, by := range s.FavedBy {
			txn.AddQuad(quad.Quad{Subject: id, Predicate: StoryFavoritedBy, Object: by})
			txn.AddQuad(quad.Quad{Subject: by, Predicate: UserFavoriteStory, Object: id})
		}
	}
	return sr.graph.ApplyTransaction(txn)
}

func recommendGeneric(s *server, urls []string, limit, offset int, reg *regexp.Regexp, site Site) (recResp, error) {
	var matches []string
	for _, url := range urls {
		if submatches := reg.FindStringSubmatch(url); len(submatches) == 2 {
			st := Story{
				Id:   atoi(submatches[1]),
				Site: site,
			}
			if st.checkExistsTitle(s.graph) {
				matches = append(matches, st.key())
			}
		}
	}
	if len(matches) > 0 {
		return recommendationStory(s, matches, limit, offset)
	}
	return recResp{}, errStoryNotFound
}

type recResp struct {
	Stories []*Story
	Authors []*User
	Story   *Story
	Stats   respStats
}

type respStats struct {
	StoryCount int
	Favorites  int
}

func recommendationStory(sr *server, keys []string, limit, offset int) (recResp, error) {
	start := time.Now()
	g := sr.graph
	s := storyByKey(g, keys[0])
	log.Printf("Finding recommendations for \"%s\"...", s.Title)
	recStories := make(map[string]float64)

	paths := []struct {
		desc string
		path *path.Path
	}{
		{
			fmt.Sprintf("%q -> %q", StoryFavoritedBy, UserFavoriteStory),
			path.StartMorphism(keys...).Out(StoryFavoritedBy).Out(UserFavoriteStory),
		},
		{
			fmt.Sprintf("%q -> %q", StoryFavoritedBy, UserStory),
			path.StartMorphism(keys...).Out(StoryFavoritedBy).Out(UserStory),
		},
		{
			fmt.Sprintf("%q -> %q", StoryAuthor, UserFavoriteStory),
			path.StartMorphism(keys...).Out(StoryAuthor).Out(UserFavoriteStory),
		},
		{
			fmt.Sprintf("%q -> %q", StoryAuthor, UserStory),
			path.StartMorphism(keys...).Out(StoryAuthor).Out(UserStory),
		},
	}
	for _, path := range paths {
		startOpt := time.Now()
		it, _ := path.path.BuildIteratorOn(g).Optimize()
		defer it.Close()
		log.Printf("%s: BuildIterator().Optimize() took %s", path.desc, time.Now().Sub(startOpt))

		startFetch := time.Now()
		for cayley.RawNext(it) {
			stID := g.NameOf(it.Result())
			recStories[stID]++
		}
		log.Printf("%s: cayley.RawNext(it) took %s", path.desc, time.Now().Sub(startFetch))
	}

	// Remove favorites pointing to original story.
	for _, key := range keys {
		delete(recStories, key)
	}

	favorites := 0
	for _, count := range recStories {
		favorites += int(count)
	}

	stories := make([]string, 0, len(recStories))
	for st := range recStories {
		stories = append(stories, st)
	}

	/*
		// Weight stories by sum of shared favorites * log(# favorites) / # favorites
		it2, _ := cayley.StartPath(g, stories...).Save(StoryFavorites, StoryFavorites).BuildIterator().Optimize()
		defer it2.Close()
		for i := 0; cayley.RawNext(it2); i++ {
			story := stories[i]
			results := map[string]graph.Value{}
			it2.TagResults(results)
			favorites := float64(atoi(g.NameOf(results[StoryFavorites])))
			if favorites == 0 {
				continue
			}
			val := recStories[story]
			recStories[story] = val * math.Log(val) / favorites
		}
	*/

	rsl := sortMap(recStories)
	if len(rsl) != len(stories) {
		log.Fatalf("len(rsl) = %d, len(stories) = %d", len(rsl), len(stories))
	}
	storyCount := len(rsl)
	if len(rsl) > (limit + offset) {
		rsl = rsl[offset : offset+limit]
	} else if len(rsl) > offset {
		rsl = rsl[offset:]
	} else {
		rsl = nil
	}
	startStories := time.Now()
	sOut := storiesByKeys(g, rsl)
	log.Printf("storiesByKeys(len = %d) took %s", len(rsl), time.Now().Sub(startStories))
	for i, st := range sOut {
		st.annotate()
		st.Score = float32(recStories[rsl[i]])
	}
	s.annotate()
	resp := recResp{
		sOut,
		nil,
		&s,
		respStats{
			storyCount,
			favorites,
		},
	}

	log.Printf("recommendationStory(%q) took %s, stats %+v", s.Title, time.Now().Sub(start), resp.Stats)
	return resp, nil
}
