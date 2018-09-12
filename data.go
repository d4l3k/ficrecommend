package main

import (
	"log"
	"regexp"
	"time"

	"github.com/dgraph-io/badger"
	"github.com/pkg/errors"
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

func (s *server) keyExists(key string) bool {
	err := s.db.View(func(txn *badger.Txn) error {
		_, err := txn.Get([]byte(key))
		return err
	})
	if err == nil {
		return true
	} else if err == badger.ErrKeyNotFound {
		return false
	}
	panic(err)
}

func (u User) checkExists(s *server) bool {
	return s.keyExists(u.key())
}

func (s Story) checkExists(sr *server) bool {
	return sr.keyExists(s.key())
}

func (s Story) checkExistsTitle(sr *server) bool {
	story, err := sr.storyByKey(s.key())
	if err == badger.ErrKeyNotFound {
		return false
	} else if err != nil {
		panic(err)
	}

	return len(story.Title) > 0
}

func (u User) key() string {
	return "user:" + Site_name[int32(u.Site)] + ":" + u.Id
}

func (u User) save(s *server) error {
	id := u.key()
	body, err := u.Marshal()
	if err != nil {
		return err
	}
	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(id), body)
	})
}

func (s *server) storyByKey(key string) (Story, error) {
	stories, err := s.storiesByKeys([]string{key})
	if err != nil {
		return Story{}, err
	}
	if len(stories) > 0 {
		return *stories[0], nil
	}
	return Story{}, errors.Errorf("not found")
}

func (s *server) storiesByKeys(keys []string) ([]*Story, error) {
	stories := make([]*Story, 0, len(keys))

	if err := s.db.View(func(txn *badger.Txn) error {
		for _, key := range keys {
			item, err := txn.Get([]byte(key))
			if err != nil {
				return err
			}
			body, err := item.Value()
			if err != nil {
				return err
			}
			s := Story{}
			if err := s.Unmarshal(body); err != nil {
				return err
			}
			s.annotate()
			stories = append(stories, &s)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return stories, nil
}

func (s *server) usersByKeys(keys []string) ([]*User, error) {
	arr := make([]*User, 0, len(keys))

	if err := s.db.View(func(txn *badger.Txn) error {
		for _, key := range keys {
			item, err := txn.Get([]byte(key))
			if err != nil {
				return err
			}
			body, err := item.Value()
			if err != nil {
				return err
			}
			v := User{}
			if err := v.Unmarshal(body); err != nil {
				return err
			}
			arr = append(arr, &v)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return arr, nil
}

func (s Story) key() string {
	return "story:" + Site_name[int32(s.Site)] + ":" + itoa(s.Id)
}

func strContains(arr []string, str string) bool {
	for _, s := range arr {
		if s == str {
			return true
		}
	}
	return false
}

func (s Story) save(sr *server) error {
	id := s.key()
	body, err := s.Marshal()
	if err != nil {
		return err
	}
	return sr.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(id), body)
	})
}

func recommendGeneric(s *server, urls []string, limit, offset int, reg *regexp.Regexp, site Site) (recResp, error) {
	var matches []string
	for _, url := range urls {
		if submatches := reg.FindStringSubmatch(url); len(submatches) == 2 {
			st := Story{
				Id:   atoi(submatches[1]),
				Site: site,
			}
			if st.checkExistsTitle(s) {
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
	Users      int
}

func recommendationStory(sr *server, keys []string, limit, offset int) (recResp, error) {
	start := time.Now()
	s, err := sr.storyByKey(keys[0])
	if err != nil {
		return recResp{}, err
	}
	log.Printf("Finding recommendations for \"%s\"...", s.Title)
	recStories := make(map[string]float64)

	// Fetch users first.
	users, err := sr.usersByKeys(s.FavedBy)
	if err != nil {
		return recResp{}, err
	}

	for _, user := range users {
		for _, story := range user.FavStories {
			recStories[story]++
		}
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
	sOut, err := sr.storiesByKeys(rsl)
	if err != nil {
		return recResp{}, err
	}
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
			len(users),
		},
	}

	log.Printf("recommendationStory(%q) took %s, stats %+v", s.Title, time.Now().Sub(start), resp.Stats)
	return resp, nil
}
