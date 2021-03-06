// Copyright © 2016 The Things Network
// Use of this source code is governed by the MIT license that can be found in the LICENSE file.

package announcement

import (
	"fmt"
	"strings"
	"time"

	"github.com/TheThingsNetwork/ttn/core/types"
	"github.com/bluele/gcache"
)

type cachedAnnouncementStore struct {
	backingStore Store
	serviceCache gcache.Cache
	listCache    gcache.Cache
}

// CacheOptions used for the cache
type CacheOptions struct {
	ServiceCacheSize       int
	ServiceCacheExpiration time.Duration
	ListCacheSize          int
	ListCacheExpiration    time.Duration
}

// DefaultCacheOptions are the default CacheOptions
var DefaultCacheOptions = CacheOptions{
	ServiceCacheSize:       1000,             // Total number of announcements to cache, thousand should be enough for now
	ServiceCacheExpiration: 10 * time.Minute, // Items be updated by ListCache fetch anyway
	ListCacheSize:          10,               // We actually only need 3: router/broker/handler
	ListCacheExpiration:    10 * time.Second, // We can afford to fetch every 10 seconds
}

func serviceCacheKey(serviceName, serviceID string) string {
	return fmt.Sprintf("%s:%s", serviceName, serviceID)
}

// NewCachedAnnouncementStore returns a cache wrapper around the existing store
func NewCachedAnnouncementStore(store Store, options CacheOptions) Store {
	serviceCache := gcache.New(options.ServiceCacheSize).Expiration(options.ServiceCacheExpiration).LFU().
		LoaderFunc(func(k interface{}) (interface{}, error) {
			key := strings.Split(k.(string), ":")
			return store.Get(key[0], key[1])
		}).Build()

	listCache := gcache.New(options.ListCacheSize).Expiration(options.ListCacheExpiration).LFU().
		LoaderFunc(func(k interface{}) (interface{}, error) {
			key := k.(string)
			announcements, err := store.ListService(key)
			if err != nil {
				return nil, err
			}
			go func(announcements []*Announcement) {
				for _, announcement := range announcements {
					serviceCache.Set(serviceCacheKey(announcement.ServiceName, announcement.ID), announcement)
				}
			}(announcements)
			return announcements, nil
		}).Build()

	return &cachedAnnouncementStore{
		backingStore: store,
		serviceCache: serviceCache,
		listCache:    listCache,
	}
}

func (s *cachedAnnouncementStore) List() ([]*Announcement, error) {
	// TODO: We're not using this function. Implement cache when we start using it.
	return s.backingStore.List()
}

func (s *cachedAnnouncementStore) ListService(serviceName string) ([]*Announcement, error) {
	l, err := s.listCache.Get(serviceName)
	if err != nil {
		return nil, err
	}
	return l.([]*Announcement), nil
}

func (s *cachedAnnouncementStore) Get(serviceName, serviceID string) (*Announcement, error) {
	a, err := s.serviceCache.Get(serviceCacheKey(serviceName, serviceID))
	if err != nil {
		return nil, err
	}
	return a.(*Announcement), nil
}

func (s *cachedAnnouncementStore) GetMetadata(serviceName, serviceID string) ([]Metadata, error) {
	a, err := s.serviceCache.Get(serviceCacheKey(serviceName, serviceID))
	if err != nil {
		return nil, err
	}
	return a.(*Announcement).Metadata, nil
}

func (s *cachedAnnouncementStore) GetForAppID(appID string) (*Announcement, error) {
	// TODO: We're not using this function. Implement cache when we start using it.
	return s.backingStore.GetForAppID(appID)
}

func (s *cachedAnnouncementStore) GetForAppEUI(appEUI types.AppEUI) (*Announcement, error) {
	// TODO: We're not using this function. Implement cache when we start using it.
	return s.backingStore.GetForAppEUI(appEUI)
}

func (s *cachedAnnouncementStore) Set(new *Announcement) error {
	if err := s.backingStore.Set(new); err != nil {
		return err
	}
	s.serviceCache.Remove(serviceCacheKey(new.ServiceName, new.ID))
	s.listCache.Remove(&new.ServiceName)
	return nil
}

func (s *cachedAnnouncementStore) AddMetadata(serviceName, serviceID string, metadata ...Metadata) error {
	if err := s.backingStore.AddMetadata(serviceName, serviceID, metadata...); err != nil {
		return err
	}
	s.serviceCache.Remove(serviceCacheKey(serviceName, serviceID))
	s.listCache.Remove(&serviceName)
	return nil
}

func (s *cachedAnnouncementStore) RemoveMetadata(serviceName, serviceID string, metadata ...Metadata) error {
	if err := s.backingStore.RemoveMetadata(serviceName, serviceID, metadata...); err != nil {
		return err
	}
	s.serviceCache.Remove(serviceCacheKey(serviceName, serviceID))
	s.listCache.Remove(&serviceName)
	return nil
}

func (s *cachedAnnouncementStore) Delete(serviceName, serviceID string) error {
	if err := s.backingStore.Delete(serviceName, serviceID); err != nil {
		return err
	}
	s.serviceCache.Remove(serviceCacheKey(serviceName, serviceID))
	s.listCache.Remove(&serviceName)
	return nil
}
