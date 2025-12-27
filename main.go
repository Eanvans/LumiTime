package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

//go:embed templates/* static/*
var content embed.FS

var (
	fansMu sync.RWMutex
	// dataStore holds persistedData per vmid
	dataMu    sync.RWMutex
	dataStore map[string]persistedData
	benchlist map[string][]string
)

type persistedData struct {
	VMID      string `json:"vmid"`
	Fans      int    `json:"fans"`
	Name      string `json:"name"`
	Avatar    string `json:"avatar"`
	CheckedAt string `json:"checked_at"`
	Platform  string `json:"platform"` // "bilibili", "youtube", "twitch"
}

// platformFetcher defines interface for fetching channel info
type platformFetcher interface {
	FetchChannelInfo(id string) (name string, avatar string, followers int, err error)
}

// saveAllData writes the entire dataStore map to the given filename atomically.
func saveAllData(filename string) error {
	dataMu.RLock()
	m := dataStore
	dataMu.RUnlock()

	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	tmp := filename + ".tmp"
	if err := os.MkdirAll(filepathDir(filename), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, filename)
}

// loadDataFile loads persisted data from path into last* variables.
// loadAllData reads all persisted files from dir (data/) into dataStore.
// loadAllData reads the single JSON file (created by saveAllData) into dataStore.
func loadAllData(filename string) error {
	dataStore = make(map[string]persistedData)
	b, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	var m map[string]persistedData
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	dataMu.Lock()
	dataStore = m
	dataMu.Unlock()
	return nil
}

// saveJSONToFile marshals v to JSON with indentation and writes it atomically to filename.
func saveJSONToFile(filename string, v interface{}) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := filename + ".tmp"
	if err := os.MkdirAll(filepathDir(filename), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, filename)
}

// loadJSONFromFile reads filename and unmarshals JSON into v (v must be a pointer).
func loadJSONFromFile(filename string, v interface{}) error {
	b, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// filepathDir returns the directory portion of a path (like path.Dir) but
// avoids importing path/filepath multiple times in other places.
func filepathDir(path string) string {
	// simple implementation: find last '/'
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			if i == 0 {
				return "/"
			}
			return path[:i]
		}
	}
	return "."
}

type biliRelationResp struct {
	Code int `json:"code"`
	Data struct {
		Mid      int `json:"mid"`
		Follower int `json:"follower"`
	} `json:"data"`
}

type nameInfo struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Avatar    string `json:"avatar"`
	Fans      int    `json:"fans"`
	CheckedAt string `json:"checked_at"`
}

type biliAccInfoResp struct {
	Code int `json:"code"`
	Data struct {
		Mid  int    `json:"mid"`
		Name string `json:"name"`
		Face string `json:"face"`
	} `json:"data"`
}

// fetchFans calls Bilibili relation API for given vmid and returns follower count
func fetchFans(vmid string) (int, error) {
	url := "https://api.bilibili.com/x/relation/stat?vmid=" + vmid
	client := &http.Client{Timeout: 10 * time.Second}
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	var br biliRelationResp
	if err := json.NewDecoder(resp.Body).Decode(&br); err != nil {
		return 0, err
	}
	return br.Data.Follower, nil
}

// fetchProfile fetches the account info (name and avatar) for the given vmid
func fetchProfile(vmid string) (string, string, error) {
	url := "https://api.bilibili.com/x/space/acc/info?mid=" + vmid
	client := &http.Client{Timeout: 10 * time.Second}
	req, _ := http.NewRequest("GET", url, nil)
	// Use a browser-like User-Agent to reduce chance of being blocked
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	// Read body so we can try robust parsing if needed
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}

	// First try decoding into expected struct
	var ar biliAccInfoResp
	if err := json.Unmarshal(body, &ar); err != nil {
		return ar.Data.Name, ar.Data.Face, nil
	}

	// If strict decode failed, try generic parsing to extract fields
	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return "", "", err
	}
	data, ok := raw["data"].(map[string]interface{})
	if !ok {
		return "", "", nil
	}
	name := ""
	avatar := ""
	if v, ok := data["name"].(string); ok {
		name = v
	}
	if v, ok := data["face"].(string); ok {
		avatar = v
	}
	return name, avatar, nil
}

// fetchYouTubeChannel fetches YouTube channel info (name, avatar, subscriber count)
// using the public channel page (web scraping)
func fetchYouTubeChannel(channelID string) (string, string, int, error) {
	url := "https://www.youtube.com/@" + channelID + "/about"
	client := &http.Client{Timeout: 15 * time.Second}
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return "", "", 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", 0, err
	}
	bodyStr := string(body)

	// Extract channel name from og:title meta tag
	name := extractRegex(bodyStr, `<meta property="og:title" content="([^"]+)"`)
	if name == "" {
		name = extractRegex(bodyStr, `<title>([^<]+)</title>`)
	}
	name = strings.TrimSpace(strings.TrimSuffix(name, "- YouTube"))

	// Extract avatar from og:image meta tag
	avatar := extractRegex(bodyStr, `<meta property="og:image" content="([^"]+)"`)

	// Extract subscriber count (look for "X subscribers" text)
	// YouTube uses various formats: "1.2M subscribers", "123K subscribers", "1,234 subscribers"
	subsText := extractRegex(bodyStr, `([0-9]+(?:[,.][0-9]+)?[KMB]?)\s*subscribers`)
	subs := parseSubscriberCount(subsText)

	return name, avatar, subs, nil
}

// fetchTwitchChannel fetches Twitch channel info (name, avatar, follower count)
// using public Twitch API or web scraping
func fetchTwitchChannel(channelName string) (string, string, int, error) {
	// Try public endpoint first (no auth required for basic info)
	url := "https://api.twitch.tv/kraken/channels/" + channelName
	client := &http.Client{Timeout: 10 * time.Second}
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Accept", "application/vnd.twitchtv.v5+json")
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := client.Do(req)
	if err == nil && resp.StatusCode == http.StatusOK {
		defer resp.Body.Close()
		var data map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&data); err == nil {
			name := ""
			avatar := ""
			followers := 0
			if v, ok := data["display_name"].(string); ok {
				name = v
			}
			if v, ok := data["logo"].(string); ok {
				avatar = v
			}
			if v, ok := data["followers"].(float64); ok {
				followers = int(v)
			}
			if name != "" && followers > 0 {
				return name, avatar, followers, nil
			}
		}
	}

	// Fallback: web scraping from Twitch profile page
	url = "https://www.twitch.tv/" + channelName
	req, _ = http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err = client.Do(req)
	if err != nil {
		return "", "", 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", 0, err
	}
	bodyStr := string(body)

	// Extract name from og:title
	name := extractRegex(bodyStr, `<meta property="og:title" content="([^"]+)"`)
	if name == "" {
		name = channelName
	}

	// Extract avatar from og:image
	avatar := extractRegex(bodyStr, `<meta property="og:image" content="([^"]+)"`)

	// Try to extract follower count from JSON-LD or other embedded data
	// Pattern: "X followers" or "X,XXX followers"
	followersText := extractRegex(bodyStr, `([0-9]+(?:,[0-9]+)*)\s*followers`)
	followers := parseFollowerCount(followersText)

	return name, avatar, followers, nil
}

// Helper: extract string using regex
func extractRegex(text, pattern string) string {
	re := regexp.MustCompile(pattern)
	matches := re.FindStringSubmatch(text)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

// Helper: parse subscriber count (e.g., "1.2M" -> 1200000)
func parseSubscriberCount(text string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}

	// Remove commas
	text = strings.ReplaceAll(text, ",", "")

	// Extract number and suffix
	matches := regexp.MustCompile(`([0-9.]+)([KMB])?`).FindStringSubmatch(text)
	if len(matches) < 2 {
		return 0
	}

	var num float64
	if n, err := fmt.Sscanf(matches[1], "%f", &num); n == 0 || err != nil {
		return 0
	}

	suffix := ""
	if len(matches) > 2 {
		suffix = matches[2]
	}

	switch suffix {
	case "K":
		return int(num * 1000)
	case "M":
		return int(num * 1000000)
	case "B":
		return int(num * 1000000000)
	default:
		return int(num)
	}
}

// Helper: parse follower count (same as subscriber count)
func parseFollowerCount(text string) int {
	return parseSubscriberCount(text)
}

// startHourlyBiliChecker starts a background goroutine that fetches
// the follower count for the given vmid immediately and then at every top-of-hour.
func startHourlyBiliChecker() {
	go func() {
		// perform check at every hour
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			for category, ids := range benchlist {
				// capture loop variables
				cat := category
				idList := ids

				// Determine fetcher based on platform category
				var fetcher func(string) (int, error)
				var fetcherProfile func(string) (string, string, error)

				switch cat {
				case "bilibili":
					fetcher = fetchFans
					fetcherProfile = fetchProfile
				case "youtube":
					fetcher = func(id string) (int, error) {
						_, _, subs, err := fetchYouTubeChannel(id)
						return subs, err
					}
					fetcherProfile = func(id string) (string, string, error) {
						name, avatar, _, err := fetchYouTubeChannel(id)
						return name, avatar, err
					}
				case "twitch":
					fetcher = func(id string) (int, error) {
						_, _, followers, err := fetchTwitchChannel(id)
						return followers, err
					}
					fetcherProfile = func(id string) (string, string, error) {
						name, avatar, _, err := fetchTwitchChannel(id)
						return name, avatar, err
					}
				default:
					log.Printf("unknown category: %s, skipping", cat)
					continue
				}

				// Process each ID in this category
				for _, id := range idList {
					time.Sleep(5 * time.Second) // slight delay between requests

					// Fetch follower count
					if v, err := fetcher(id); err == nil {
						dataMu.Lock()
						d := dataStore[id]
						d.VMID = id
						d.Fans = v
						d.CheckedAt = time.Now().Format(time.RFC3339)
						d.Platform = cat
						dataStore[id] = d
						dataMu.Unlock()
						log.Printf("[%s] %s fans=%d", cat, id, v)
						if err := saveAllData("data.json"); err != nil {
							log.Printf("save data failed: %v", err)
						}
					} else {
						log.Printf("[%s] %s fetch failed: %v", cat, id, err)
					}

					time.Sleep(2 * time.Second) // slight delay between requests

					// Fetch profile info
					if name, face, err := fetcherProfile(id); err == nil {
						dataMu.Lock()
						d := dataStore[id]
						d.VMID = id
						if name != "" {
							d.Name = name
						}
						if face != "" {
							d.Avatar = face
						}
						d.Platform = cat
						dataStore[id] = d
						dataMu.Unlock()
						log.Printf("[%s] %s profile name=%s", cat, id, name)
						if err := saveAllData("data.json"); err != nil {
							log.Printf("save data failed: %v", err)
						}
					} else {
						log.Printf("[%s] %s profile fetch failed: %v", cat, id, err)
					}
				}
			}
			// wait for next tick
			<-ticker.C
		}
	}()
}

// loadBenchlist reads benchlist.json from given path and returns a map of category -> ids
func loadBenchlist(path string) (map[string][]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m map[string][]string
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func main() {
	r := gin.Default()

	tmpl := template.Must(template.ParseFS(content, "templates/*"))
	r.SetHTMLTemplate(tmpl)

	// Serve embedded static files under /static (use fs.Sub to set the static/ folder as FS root)
	staticFiles, err := fs.Sub(content, "static")
	if err != nil {
		panic(err)
	}
	r.StaticFS("/static", http.FS(staticFiles))

	// Load benchlist.json from current working directory (if present)
	if bl, err := loadBenchlist("benchlist.json"); err == nil {
		benchlist = bl
		log.Printf("loaded benchlist categories: %v", func() []string {
			keys := make([]string, 0, len(benchlist))
			for k := range benchlist {
				keys = append(keys, k)
			}
			return keys
		}())
	} else {
		log.Printf("benchlist.json not loaded: %v", err)
		benchlist = make(map[string][]string)
	}

	// Try to load persisted data from data.json if present
	if err := loadAllData("data.json"); err == nil {
		dataMu.RLock()
		keys := make([]string, 0, len(dataStore))
		for k := range dataStore {
			keys = append(keys, k)
		}
		dataMu.RUnlock()
		log.Printf("loaded persisted data for ids: %v", keys)
	} else {
		log.Printf("no persisted data loaded: %v", err)
	}

	// register API routes
	registerAPIs(r)

	// check tuber status hourlys
	//startHourlyBiliChecker()

	fetchYouTubeChannel("KanekoLumi")

	// Listen on :8080
	r.Run(":8080")
}
