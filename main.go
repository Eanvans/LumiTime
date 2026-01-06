package main

import (
	"log"
	"sync"
	"time"

	"subtuber-services/handlers"
	"subtuber-services/services"

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
		RPC    handlers.RPCConfig    `mapstructure:"rpc"`
	}
	_ = viper.Unmarshal(&cfg)

	// set default timeout if not provided
	if cfg.SMTP.Timeout == 0 {
		cfg.SMTP.Timeout = 30 * time.Second
	}
	handlers.SetSMTPConfig(cfg.SMTP)
	handlers.SetRPCConfig(cfg.RPC)

	// 初始化 RPC 服务（可选，如果配置了 RPC 地址）
	if cfg.RPC.Address != "" {
		timeout := time.Duration(cfg.RPC.TimeoutSeconds) * time.Second
		if timeout == 0 {
			timeout = 10 * time.Second
		}

		streamerServiceCfg := services.StreamerServiceConfig{
			RPCAddress: cfg.RPC.Address,
			Timeout:    timeout,
		}

		_, err := services.InitStreamerService(streamerServiceCfg)
		if err != nil {
			log.Printf("警告: 无法初始化 RPC 服务: %v", err)
			log.Println("系统将在没有 RPC 功能的情况下继续运行")
		}
	}

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

	//testGenaiAPI(_googleAiApiKey)

	// Listen on :8080
	r.Run(":8080")
}
