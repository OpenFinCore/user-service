package repository

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"ofc-user-service/conf"
)

var (
	redisOnce   sync.Once
	redisClient *redis.Client
)

// SharedRedis returns the process-wide Redis client, connecting (and
// verifying the connection with PING) on first use. It panics if Redis is
// unreachable at startup: the service must not run without it.
func SharedRedis() *redis.Client {
	redisOnce.Do(func() {
		cfg := conf.GetConfig().Redis
		client := redis.NewClient(&redis.Options{
			Addr:         cfg.Addr,
			Password:     cfg.Password,
			DB:           cfg.DB,
			DialTimeout:  5 * time.Second,
			ReadTimeout:  3 * time.Second,
			WriteTimeout: 3 * time.Second,
		})

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := client.Ping(ctx).Err(); err != nil {
			panic(fmt.Errorf("connecting to redis at %s: %w", cfg.Addr, err))
		}
		redisClient = client
	})
	return redisClient
}
