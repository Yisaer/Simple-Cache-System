package GoCache

import (
	"encoding/gob"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

type Item struct {
	Object     interface{}
	Expiration int64
}

const (
	NoExpiration time.Duration = -1

	DefaultExpiration time.Duration = 0
)

type Cache struct {
	defaultExpiration time.Duration
	items             map[string]Item // Cache in map
	mutex             sync.RWMutex
	gcInterval        time.Duration
	stopGc            chan bool
}

//Check Data if Expired

func (item Item) Expired() bool {

	if item.Expiration == 0 {
		return false
	}
	return time.Now().UnixNano() > item.Expiration
}

// Clear Data in Cache
func (c *Cache) gcLoop() {
	ticker := time.NewTicker(c.gcInterval)
	for {
		select {
		case <-ticker.C:
			c.DeleteExpired()
		case <-c.stopGc:
			ticker.Stop()
			return
		}
	}
}

//Delete Cache Data
func (c *Cache) delete(k string) {
	delete(c.items, k)
}

// Trans All Data in Map And Delete Expired Data
func (c *Cache) DeleteExpired() {

	now := time.Now().UnixNano()
	c.mutex.Lock()
	defer c.mutex.Unlock()
	for k, v := range c.items {
		if v.Expiration > 0 && now > v.Expiration {
			c.delete(k)
		}
	}
}

// To Set the Data

func (c *Cache) Set(k string, v interface{}, d time.Duration) {
	var e int64
	if d == DefaultExpiration {
		d = c.defaultExpiration
	}
	if d > 0 {
		e = time.Now().Add(d).UnixNano()
	}
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.items[k] = Item{
		Object:     v,
		Expiration: e,
	}

}

// To Get the Data

func (c *Cache) Get(k string) (interface{}, bool) {
	item, found := c.items[k]
	if !found {
		return nil, false
	}
	if item.Expired() {
		return nil, false
	}
	return item.Object, true
}

// Add Data if it did not Exist yet
func (c *Cache) Add(k string, v interface{}, d time.Duration) error {
	c.mutex.Lock()
	_, found := c.Get(k)
	if found {
		c.mutex.Unlock()
		return fmt.Errorf("item %s already exists", k)
	}
	c.Set(k, v, d)
	c.mutex.Unlock()
	return nil
}

func (c *Cache) Replace(k string, v interface{}, d time.Duration) error {
	c.mutex.Lock()
	_, found := c.Get(k)
	if !found {
		c.mutex.Unlock()
		return fmt.Errorf("Item %s doesnt Exist", k)
	}
	c.Set(k, v, d)
	c.mutex.Unlock()
	return nil
}

//Delete ... obviousely
func (c *Cache) Delete(k string) {
	c.mutex.Lock()
	c.delete(k)
	c.mutex.Unlock()
}

// Save ... Let Cache Write In WriteIO
func (c *Cache) Save(w io.Writer) (err error) {
	enc := gob.NewEncoder(w)
	defer func() {
		if x := recover(); x != nil {
			err = fmt.Errorf("Error registering item types with Gob lib")
		}
	}()
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	for _, v := range c.items {
		gob.Register(v.Object)
	}
	err = enc.Encode(&c.items)
	return
}

//SaveToFile ... obviously Too
func (c *Cache) SaveToFile(file string) error {
	f, err := os.Create(file)
	if err != nil {
		return err
	}
	if err = c.Save(f); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

//Load ... Load Data IN ioReader
// We use gob to deserializatize the data in ioReader
// And Find the object with key in ReturnedItem
func (c *Cache) Load(r io.Reader) error {
	dec := gob.NewDecoder(r)
	items := map[string]Item{}
	err := dec.Decode(&items)
	if err == nil {
		c.mutex.Lock()
		defer c.mutex.Unlock()
		for k, v := range items {
			ov, found := c.items[k]
			if !found || ov.Expired() {
				c.items[k] = v
			}
		}
	}
	return err
}

//LoadFile ... Load Cache From File
func (c *Cache) LoadFile(file string) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	if err = c.Load(f); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

//Count ... Return Number of Data In Cache
func (c *Cache) Count() int {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return len(c.items)
}

//Flush .. Flush the Cache
func (c *Cache) Flush() {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.items = map[string]Item{}
}

func (c *Cache) StopGc() {
	c.stopGc <- true
}

//NewCache ... Create a New Cache System And goRoutine
func NewCache(defaultExpiration, gcInterval time.Duration) *Cache {
	c := &Cache{
		defaultExpiration: defaultExpiration,
		gcInterval:        gcInterval,
		items:             map[string]Item{},
		stopGc:            make(chan bool),
	}
	go c.gcLoop()
	return c
}
