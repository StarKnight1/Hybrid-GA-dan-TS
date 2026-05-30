package main

import (
	"log"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"

	"smp_mater_dei_be/internal/admin"
	"smp_mater_dei_be/internal/platform/config"
	"smp_mater_dei_be/internal/platform/database/migrations"
	"smp_mater_dei_be/internal/platform/database/seeders"
	"smp_mater_dei_be/internal/platform/logging"
	savedschedules "smp_mater_dei_be/internal/saved_schedules"
	"smp_mater_dei_be/internal/schedule"
	"smp_mater_dei_be/internal/transport/http/middleware"
	"smp_mater_dei_be/internal/users"
)

func main() {
	_ = godotenv.Load()

	logging.InitLogger()
	config.InitDB()

	migrations.Run()
	if err := seeders.SeedTemp(config.DB); err != nil {
		log.Fatal(err)
	}

	r := gin.New()
	r.Use(middleware.CORS())
	r.Use(middleware.ZapLogger())
	r.Use(gin.Recovery())

	users.RegisterRoutes(r)
	schedule.RegisterRoutes(r)
	admin.RegisterRoutes(r)
	savedschedules.RegisterRoutes(r)

	log.Fatal(r.Run(":" + config.AppPort()))
}
