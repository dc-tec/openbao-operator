package openbao

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"golang.org/x/sync/singleflight"
	"k8s.io/apimachinery/pkg/util/cache"
)

const jwtTokenCacheSkew = 10 * time.Second

type jwtTokenLoginFunc func(context.Context) (token string, ttlSeconds int, err error)

type jwtTokenCache struct {
	cache *cache.Expiring
	group singleflight.Group
}

func newJWTTokenCache() *jwtTokenCache {
	return &jwtTokenCache{cache: cache.NewExpiring()}
}

func (c *jwtTokenCache) getOrLogin(ctx context.Context, baseURL, role, jwtToken string, login jwtTokenLoginFunc) (string, error) {
	if c == nil || c.cache == nil {
		return loginWithoutCache(ctx, login)
	}

	key := jwtTokenCacheKey(baseURL, role, jwtToken)
	if token, ok := c.get(key); ok {
		return token, nil
	}

	value, err, _ := c.group.Do(key, func() (any, error) {
		if token, ok := c.get(key); ok {
			return token, nil
		}

		token, ttlSeconds, err := login(ctx)
		if err != nil {
			return "", err
		}
		ttl := jwtTokenCacheTTL(ttlSeconds)
		if ttl > 0 {
			c.cache.Set(key, token, ttl)
		}
		return token, nil
	})
	if err != nil {
		return "", err
	}

	token, ok := value.(string)
	if !ok || token == "" {
		return "", fmt.Errorf("JWT auth returned empty token")
	}
	return token, nil
}

func (c *jwtTokenCache) get(key string) (string, bool) {
	value, ok := c.cache.Get(key)
	if !ok {
		return "", false
	}
	token, ok := value.(string)
	return token, ok && token != ""
}

func loginWithoutCache(ctx context.Context, login jwtTokenLoginFunc) (string, error) {
	token, _, err := login(ctx)
	if err != nil {
		return "", err
	}
	if token == "" {
		return "", fmt.Errorf("JWT auth returned empty token")
	}
	return token, nil
}

func jwtTokenCacheTTL(ttlSeconds int) time.Duration {
	if ttlSeconds <= 0 {
		return 0
	}

	ttl := time.Duration(ttlSeconds) * time.Second
	if ttl <= jwtTokenCacheSkew {
		return 0
	}
	return ttl - jwtTokenCacheSkew
}

func jwtTokenCacheKey(baseURL, role, jwtToken string) string {
	sum := sha256.Sum256([]byte(jwtToken))
	return baseURL + "\x00" + role + "\x00" + hex.EncodeToString(sum[:])
}
