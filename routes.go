package main

import (
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/gin-gonic/gin"
)

// registerAPIs registers HTTP handlers on the provided gin Engine.
func registerAPIs(r *gin.Engine) {
	r.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.html", gin.H{"Title": "LumiTime - 内嵌页面示例"})
	})

	r.GET("/api/time", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"time": time.Now().Format(time.RFC3339)})
	})

	// expose benchlist for inspection
	r.GET("/api/benchlist", func(c *gin.Context) {
		c.JSON(http.StatusOK, benchlist)
	})

	// Expose names list as full objects: id, name, avatar, fans, checked_at
	r.GET("/api/names", func(c *gin.Context) {
		infos := make([]nameInfo, 0)

		if ids, ok := benchlist["bilibili"]; ok && len(ids) > 0 {
			for _, id := range ids {
				dataMu.RLock()
				d, found := dataStore[id]
				dataMu.RUnlock()

				ni := nameInfo{ID: id}
				if found {
					ni.Name = d.Name
					ni.Avatar = d.Avatar
					ni.Fans = d.Fans
					ni.CheckedAt = d.CheckedAt
				}
				infos = append(infos, ni)
			}
		}

		if len(infos) == 0 {
			// fallback to a simple default list
			infos = append(infos, nameInfo{ID: "", Name: "Unknown", Avatar: "", Fans: 0, CheckedAt: ""})
		}

		c.JSON(http.StatusOK, infos)
	})

	// Image proxy to avoid CORS issues for external avatar URLs.
	// Only allow a small whitelist of hosts to reduce abuse.
	r.GET("/img/proxy", func(c *gin.Context) {
		raw := c.Query("url")
		if raw == "" {
			c.Status(http.StatusBadRequest)
			return
		}
		u, err := url.Parse(raw)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
			c.Status(http.StatusBadRequest)
			return
		}

		// whitelist by hostname
		allowed := map[string]bool{
			"i0.hdslb.com":     true,
			"i1.hdslb.com":     true,
			"i2.hdslb.com":     true,
			"i3.hdslb.com":     true,
			"api.dicebear.com": true,
		}
		host := u.Hostname()
		if !allowed[host] {
			c.Status(http.StatusForbidden)
			return
		}

		client := &http.Client{Timeout: 10 * time.Second}
		req, _ := http.NewRequest("GET", raw, nil)
		req.Header.Set("User-Agent", "Mozilla/5.0 (compatible)")
		resp, err := client.Do(req)
		if err != nil {
			c.Status(http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		// forward content-type and allow cross-origin
		if ct := resp.Header.Get("Content-Type"); ct != "" {
			c.Header("Content-Type", ct)
		}
		c.Header("Access-Control-Allow-Origin", "*")

		// stream body
		c.Status(resp.StatusCode)
		io.Copy(c.Writer, resp.Body)
	})
}
