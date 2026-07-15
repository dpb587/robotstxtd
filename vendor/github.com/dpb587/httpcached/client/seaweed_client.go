package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type seaweedClient struct {
	log           *slog.Logger
	blobTransport http.RoundTripper

	collectionVolumeLookups      map[string]seaweedfsCollectionVolumeLookups
	collectionVolumeLookupsMutex sync.RWMutex
}

type seaweedfsCollectionVolumeLookups struct {
	refreshTime time.Time
	locations   []string
}

func NewSeaweedClient(log *slog.Logger, blobTransport *http.Transport) *seaweedClient {
	c := &seaweedClient{
		log:                     log,
		blobTransport:           blobTransport,
		collectionVolumeLookups: map[string]seaweedfsCollectionVolumeLookups{},
	}

	if c.blobTransport == nil {
		c.blobTransport = http.DefaultTransport
	}

	return c
}

func (c *seaweedClient) NewRequest(ctx context.Context, method string, uri string) (*http.Request, error) {
	uriParsed, err := url.Parse(uri)
	if err != nil {
		return nil, fmt.Errorf("parse uri: %v", err)
	} else if uriParsed.Scheme != "seaweedfs" {
		return nil, fmt.Errorf("unexpected scheme: %s", uriParsed.Scheme)
	}

	fileId := strings.SplitN(strings.TrimPrefix(uriParsed.Path, "/"), "/", 2)[0]
	volumeId := strings.SplitN(fileId, ",", 2)[0]

	locations, err := c.getLocations(uriParsed.Host, volumeId, uriParsed.Query().Get("collection"))
	if err != nil {
		return nil, fmt.Errorf("get locations: %v", err)
	} else if len(locations) == 0 {
		return nil, errors.New("no locations found")
	}

	return http.NewRequestWithContext(ctx, method, "http://"+locations[0]+"/"+fileId, nil)
}

func (c *seaweedClient) GetObject(ctx context.Context, uri string) (io.ReadCloser, error) {
	req, err := c.NewRequest(ctx, http.MethodGet, uri)
	if err != nil {
		return nil, fmt.Errorf("create request: %v", err)
	}

	cachedContent, err := c.blobTransport.RoundTrip(req)
	if err != nil {
		return nil, fmt.Errorf("get: %v", err)
	} else if cachedContent.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", cachedContent.StatusCode)
	}

	return cachedContent.Body, nil
}

func (c *seaweedClient) getRefKey(masterHost, volumeId string) string {
	return masterHost + "/" + volumeId
}

func (c *seaweedClient) getLocations(masterHost, volumeId, collectionHint string) ([]string, error) {
	c.collectionVolumeLookupsMutex.RLock()
	lookup, ok := c.collectionVolumeLookups[c.getRefKey(masterHost, volumeId)]
	c.collectionVolumeLookupsMutex.RUnlock()

	if !ok || time.Now().After(lookup.refreshTime) {
		return c.getFreshLocations(masterHost, volumeId, collectionHint)
	}

	return lookup.locations, nil
}

func (c *seaweedClient) getFreshLocations(masterHost, volumeId, collectionHint string) ([]string, error) {
	refKey := c.getRefKey(masterHost, volumeId)

	c.collectionVolumeLookupsMutex.Lock()
	defer c.collectionVolumeLookupsMutex.Unlock()

	// avoid repeated, queued lookups
	lookup, ok := c.collectionVolumeLookups[refKey]
	if ok && lookup.refreshTime.After(time.Now()) {
		return lookup.locations, nil
	}

	c.log.Info("seaweedfs: load volume locations", "key", refKey)

	// stale by default
	nextLookup := seaweedfsCollectionVolumeLookups{
		refreshTime: time.Now().Add(60 * time.Second),
		locations:   lookup.locations,
	}

	defer func() {
		c.collectionVolumeLookups[refKey] = nextLookup
	}()

	ctx, ctxCancel := context.WithDeadline(context.Background(), time.Now().Add(10*time.Second))
	defer ctxCancel()

	lookupRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://%s/dir/lookup?collection=%s&volumeId=%s", masterHost, collectionHint, volumeId), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %v", err)
	}

	lookupResponse, err := c.blobTransport.RoundTrip(lookupRequest)
	if err != nil {
		return nil, fmt.Errorf("get: %v", err)
	} else if lookupResponse.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", lookupResponse.StatusCode)
	}

	defer lookupResponse.Body.Close()

	var lookupResponseData struct {
		Locations []struct {
			Url       string `json:"url"`
			PublicUrl string `json:"publicUrl"`
		} `json:"locations"`
	}

	if err := json.NewDecoder(lookupResponse.Body).Decode(&lookupResponseData); err != nil {
		return nil, fmt.Errorf("decode response: %v", err)
	}

	nextLookup.locations = []string{}

	for _, location := range lookupResponseData.Locations {
		nextLookup.locations = append(nextLookup.locations, location.PublicUrl)
	}

	nextLookup.refreshTime = time.Now().Add(15 * time.Minute)

	return nextLookup.locations, nil
}
