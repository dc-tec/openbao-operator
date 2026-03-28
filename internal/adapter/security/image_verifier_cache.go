package security

import (
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/hashicorp/golang-lru/v2/expirable"
)

const (
	defaultVerificationCacheSize = 256
	defaultTagCacheSize          = 128
	defaultTagCacheTTL           = 5 * time.Minute
)

type verificationCache struct {
	cache *lru.Cache[string, struct{}]
}

func newVerificationCache() *verificationCache {
	cache, _ := lru.New[string, struct{}](defaultVerificationCacheSize)
	return &verificationCache{cache: cache}
}

func (c *verificationCache) isVerifiedByKey(cacheKey string) bool {
	_, ok := c.cache.Get(cacheKey)
	return ok
}

func (c *verificationCache) markVerifiedByKey(cacheKey string) {
	c.cache.Add(cacheKey, struct{}{})
}

type tagResolutionCache struct {
	cache *expirable.LRU[string, string]
}

func newTagResolutionCache() *tagResolutionCache {
	return &tagResolutionCache{
		cache: expirable.NewLRU[string, string](defaultTagCacheSize, nil, defaultTagCacheTTL),
	}
}

func (c *tagResolutionCache) get(imageRef string) (string, bool) {
	return c.cache.Get(imageRef)
}

func (c *tagResolutionCache) set(imageRef, digest string) {
	c.cache.Add(imageRef, digest)
}
