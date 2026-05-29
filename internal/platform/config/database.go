package config

import (
	"context"
	"fmt"
	"os"
	"smp_mater_dei_be/internal/platform/logging"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var DB *gorm.DB
var RedisSystem *redis.Client
var RedisSession *redis.Client

func InitDB() {
	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=%s",
		os.Getenv("DB_HOST"),
		os.Getenv("DB_USER"),
		os.Getenv("DB_PASS"),
		os.Getenv("DB_NAME"),
		os.Getenv("DB_PORT"),
		os.Getenv("DB_SSLMODE"),
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		logging.Logger.Fatal("failed to connect to postgres", zap.Error(err))
	}

	DB = db
}

func initRedisClient(dbNum int) *redis.Client {
	rdb := redis.NewClient(&redis.Options{
		Addr:     os.Getenv("REDIS_HOST") + ":" + os.Getenv("REDIS_PORT"),
		Password: os.Getenv("REDIS_PASS"),
		DB:       dbNum,
	})

	if err := rdb.Ping(context.Background()).Err(); err != nil {
		logging.Logger.Fatal("failed to connect to redis", zap.Int("db", dbNum), zap.Error(err))
	}

	return rdb
}

func InitRedis() {
	RedisSystem = initRedisClient(0)
	RedisSession = initRedisClient(1)
}
