package main

import (
	"sync"
	"time"

	"subtuber-services/handlers"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
)

var (
	// dataStore holds persistedData per vmid
	dataMu          sync.RWMutex
	_googleAiApiKey = "AIzaSyBuz5ddmuj7ykpSdIjjHtDJea1Y2M5p7yQ"
)

func main() {
	// load configuration (config.yaml) via viper
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	_ = viper.ReadInConfig()

	var cfg struct {
		SMTP   handlers.SMTPConfig   `mapstructure:"smtp"`
		Twitch handlers.TwitchConfig `mapstructure:"twitch"`
	}
	_ = viper.Unmarshal(&cfg)
	// set default timeout if not provided
	if cfg.SMTP.Timeout == 0 {
		cfg.SMTP.Timeout = 30 * time.Second
	}
	handlers.SetSMTPConfig(cfg.SMTP)

	// 初始化并启动Twitch监控服务
	if cfg.Twitch.ClientID != "" && cfg.Twitch.StreamerName != "" {
		twitchMonitor := handlers.InitTwitchMonitor(cfg.Twitch)
		twitchMonitor.Start()
	}

	r := gin.Default()

	// CORS middleware for frontend development
	r.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})

	// register legacy API routes
	registerAPIs(r)

	// register new structured API routes
	//router.SetupRouter(r)

	//testGenaiAPI(_googleAiApiKey)

	// Listen on :8080
	r.Run(":8080")
}
