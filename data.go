package main

import (
	"errors"
	"log"
	"strconv"

	"github.com/cayleygraph/cayley"
	"github.com/cayleygraph/cayley/graph"
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
	s := Story{}
	it, _ := cayley.StartPath(g, key).Save(StoryID, StoryID).Save(StoryTitle, StoryTitle).Save(StoryDescription, StoryDescription).Save(StoryURL, StoryURL).Save(SiteID, SiteID).BuildIterator().Optimize()
	defer it.Close()
	for cayley.RawNext(it) {
		results := map[string]graph.Value{}
		it.TagResults(results)
		s.Id = atoi(g.NameOf(results[StoryID]))
		s.Title = g.NameOf(results[StoryTitle])
		s.Desc = g.NameOf(results[StoryDescription])
		s.Url = g.NameOf(results[StoryURL])
		s.Site = Site(Site_value[g.NameOf(results[SiteID])])
	}
	return s
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
		txn.AddQuad(quad.Quad{Subject: id, Predicate: StoryComplete, Object: strconv.FormatBool(s.Complete)})
		for _, by := range s.FavedBy {
			txn.AddQuad(quad.Quad{Subject: id, Predicate: StoryFavoritedBy, Object: by})
			txn.AddQuad(quad.Quad{Subject: by, Predicate: UserFavoriteStory, Object: id})
		}
	}
	return sr.graph.ApplyTransaction(txn)
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

func recommendationStory(sr *server, s Story, limit, offset int) (recResp, error) {
	var sOut []*Story
	var uOut []*User
	if !s.checkExists(sr.graph) {
		return recResp{}, errStoryNotFound
	}
	s = storyByKey(sr.graph, s.key())
	log.Printf("Finding recommendations for \"%s\"...", s.Title)
	recStories := make(map[string]int)

	it, _ := cayley.StartPath(sr.graph, s.key()).Out(StoryFavoritedBy).In(StoryFavoritedBy).BuildIterator().Optimize()
	defer it.Close()

	for cayley.RawNext(it) {
		stID := sr.graph.NameOf(it.Result())
		recStories[stID]++
	}

	favorites := 0
	for _, count := range recStories {
		favorites += count
	}

	rsl := sortMap(recStories, limit+offset)
	if len(rsl) > 1 && rsl[0] == s.key() {
		rsl = rsl[1:]
	}
	storyCount := len(rsl)
	if len(rsl) > (limit + offset) {
		rsl = rsl[offset : offset+limit]
	} else if len(rsl) > offset {
		rsl = rsl[offset:]
	} else {
		rsl = nil
	}
	sOut = fetchStories(sr.graph, rsl)
	for _, st := range sOut {
		st.FavedBy = nil
		st.annotate()
	}
	for _, st := range uOut {
		st.FavedBy = nil
	}
	s.annotate()
	resp := recResp{
		sOut,
		uOut,
		&s,
		respStats{
			storyCount,
			favorites,
		},
	}
	return resp, nil
}
