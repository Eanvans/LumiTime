package main

import (
	"embed"
	"encoding/json"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
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

// startHourlyBiliChecker starts a background goroutine that fetches
// the follower count for the given vmid immediately and then at every top-of-hour.
func startHourlyBiliChecker() {
	go func() {
		for category, ids := range benchlist {
			// capture loop variables
			cat := category
			idList := ids
			if cat == "bilibili" {
				for _, vmid := range idList {
					time.Sleep(5 * time.Second) // slight delay between requests

					// Start background checker for Bilibili fans
					// optional: do an immediate fetch so we have initial data
					if v, err := fetchFans(vmid); err == nil {
						// update dataStore
						dataMu.Lock()
						d := dataStore[vmid]
						d.VMID = vmid
						d.Fans = v
						d.CheckedAt = time.Now().Format(time.RFC3339)
						dataStore[vmid] = d
						dataMu.Unlock()
						log.Printf("initial fans=%d", v)
						if err := saveAllData("data.json"); err != nil {
							log.Printf("save data failed: %v", err)
						}
					} else {
						log.Printf("initial fetch failed: %v", err)
					}

					time.Sleep(2 * time.Second) // slight delay between requests

					if name, face, err := fetchProfile(vmid); err == nil {
						dataMu.Lock()
						d := dataStore[vmid]
						d.VMID = vmid
						if name != "" {
							d.Name = name
						}
						if face != "" {
							d.Avatar = face
						}
						dataStore[vmid] = d
						dataMu.Unlock()
						log.Printf("initial profile name=%s avatar=%s", name, face)
						if err := saveAllData("data.json"); err != nil {
							log.Printf("save data failed: %v", err)
						}
					} else {
						log.Printf("initial profile fetch failed: %v", err)
					}
				}
			}
		}

		// // perform check at every hour
		// ticker := time.NewTicker(time.Hour)
		// defer ticker.Stop()
		// for {

		// 	// wait for next tick
		// 	<-ticker.C
		// }
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
	startHourlyBiliChecker()

	// Listen on :8080
	r.Run(":8080")
}
