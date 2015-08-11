package main

import (
	"log"
	"strconv"

	"github.com/boltdb/bolt"
	"github.com/golang/protobuf/proto"
)

func userByKey(key string) *User {
	return &User{
		Id:   key[2:],
		Site: Site(atoi(key[:1])),
	}
}

func (u *User) update(db *bolt.DB) (found bool) {
	db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("users"))
		v := b.Get(u.key())
		if len(v) == 0 {
			return nil
		}
		found = true
		return proto.Unmarshal(v, u)
	})
	return
}
func (u *User) key() []byte {
	return []byte(strconv.Itoa(int(u.Site)) + ":" + u.Id)
}
func (u *User) save(db *bolt.DB) error {
	return db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("users"))
		encoded, err := proto.Marshal(u)
		if err != nil {
			return err
		}
		return b.Put(u.key(), encoded)
	})
}

func (u *User) updateBucket(b *bolt.Bucket) error {
	v := b.Get(u.key())
	if len(v) == 0 {
		return nil
	}
	return proto.Unmarshal(v, u)
}

func storyByKey(key string) *Story {
	return &Story{
		Id:   atoi(key[2:]),
		Site: Site(atoi(key[:1])),
	}
}

func (s *Story) key() []byte {
	return []byte(strconv.Itoa(int(s.Site)) + ":" + itoa(s.Id))
}

func (s *Story) update(db *bolt.DB) (found bool) {
	db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("stories"))
		v := b.Get(s.key())
		if len(v) == 0 {
			return nil
		}
		found = true
		return proto.Unmarshal(v, s)
	})
	return
}

func (s *Story) updateBucket(b *bolt.Bucket) error {
	v := b.Get(s.key())
	if len(v) == 0 {
		return nil
	}
	return proto.Unmarshal(v, s)
}

func (s *Story) save(db *bolt.DB) error {
	return db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("stories"))
		encoded, err := proto.Marshal(s)
		if err != nil {
			return err
		}
		return b.Put(s.key(), encoded)
	})
}

func recommendationStory(db *bolt.DB, s *Story, limit int) *recResp {
	var sOut []*Story
	var uOut []*User
	if !s.update(db) {
		log.Println("Failed to find story.")
		return nil
	}
	log.Printf("Finding recommendations for \"%s\"...", s.Title)
	usersSeen := make(map[string]bool)
	recStories := make(map[string]int)
	//recAuthors := make(map[int]int)
	db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("users"))
		bstories := tx.Bucket([]byte("stories"))
		for _, userID := range s.FavedBy {
			if usersSeen[userID] {
				continue
			}
			usersSeen[userID] = true

			u := userByKey(userID)
			u.updateBucket(b)
			for _, storyID := range u.FavStories {
				recStories[storyID]++
			}
			/*
				for _, authorId := range u.FavAuthors {
					recAuthors[authorId]++
				}*/
		}
		//ral := sortMap(recAuthors, limit)
		rsl := sortMap(recStories, limit)
		if len(rsl) > 1 {
			rsl = rsl[1:]
		}
		if len(rsl) > limit {
			rsl = rsl[:limit]
		}
		/*
			if len(ral) > limit {
				ral = ral[:limit]
			}*/
		sOut = fetchStories(bstories, rsl)
		//uOut = fetchUsers(b, ral)
		return nil
	})
	for _, st := range sOut {
		st.FavedBy = nil
		st.annotate()
	}
	for _, st := range uOut {
		st.FavedBy = nil
	}
	s.annotate()
	resp := &recResp{
		sOut,
		uOut,
		s,
	}
	return resp
}
