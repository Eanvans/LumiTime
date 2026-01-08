package services

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"net/http"
	"time"
)

// BcutASR 必剪（Bilibili）语音识别服务
type BcutASR struct {
	fileBinary  []byte
	crc32Hex    string
	taskID      string
	inBossKey   string
	resourceID  string
	uploadID    string
	uploadURLs  []string
	perSize     int
	clips       int
	etags       []string
	downloadURL string
}

// ASRSegment 字幕片段
type ASRSegment struct {
	Text      string `json:"text"`
	StartTime int64  `json:"start_time"`
	EndTime   int64  `json:"end_time"`
}

// ASRResult 识别结果
type ASRResult struct {
	Segments []ASRSegment `json:"segments"`
	RawData  interface{}  `json:"raw_data"`
}

// API endpoints
const (
	APIBaseURL      = "https://member.bilibili.com/x/bcut/rubick-interface"
	APIReqUpload    = APIBaseURL + "/resource/create"
	APICommitUpload = APIBaseURL + "/resource/create/complete"
	APICreateTask   = APIBaseURL + "/task"
	APIQueryResult  = APIBaseURL + "/task/result"
)

// NewBcutASR 创建必剪ASR实例
func NewBcutASR(audioData []byte) *BcutASR {
	crc32Value := crc32.ChecksumIEEE(audioData)
	crc32Hex := fmt.Sprintf("%08x", crc32Value)

	return &BcutASR{
		fileBinary: audioData,
		crc32Hex:   crc32Hex,
		etags:      make([]string, 0),
	}
}

// buildHeaders 构建请求头
func (b *BcutASR) buildHeaders() map[string]string {
	return map[string]string{
		"User-Agent":   "Bilibili/1.0.0 (https://www.bilibili.com)",
		"Content-Type": "application/json",
	}
}

// requestUpload 申请上传
func (b *BcutASR) requestUpload() error {
	payload := map[string]interface{}{
		"type":             2,
		"name":             "audio.mp3",
		"size":             len(b.fileBinary),
		"ResourceFileType": "mp3",
		"model_id":         "8",
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", APIReqUpload, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	headers := b.buildHeaders()
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("request upload failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("failed to parse JSON response: %w", err)
	}

	data, ok := result["data"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid response, no data field: %s", string(body))
	}

	b.inBossKey = data["in_boss_key"].(string)
	b.resourceID = data["resource_id"].(string)
	b.uploadID = data["upload_id"].(string)
	b.perSize = int(data["per_size"].(float64))

	uploadURLsRaw := data["upload_urls"].([]interface{})
	b.uploadURLs = make([]string, len(uploadURLsRaw))
	for i, url := range uploadURLsRaw {
		b.uploadURLs[i] = url.(string)
	}
	b.clips = len(b.uploadURLs)

	fmt.Printf("申请上传成功, 总计大小%dKB, %d分片, 分片大小%dKB: %s\n",
		len(b.fileBinary)/1024, b.clips, b.perSize/1024, b.inBossKey)

	return nil
}

// uploadParts 上传音频数据分片
func (b *BcutASR) uploadParts() error {
	for i := 0; i < b.clips; i++ {
		startRange := i * b.perSize
		endRange := (i + 1) * b.perSize
		if endRange > len(b.fileBinary) {
			endRange = len(b.fileBinary)
		}

		fmt.Printf("开始上传分片%d: %d-%d\n", i, startRange, endRange)

		req, err := http.NewRequest("PUT", b.uploadURLs[i],
			bytes.NewBuffer(b.fileBinary[startRange:endRange]))
		if err != nil {
			return err
		}

		headers := b.buildHeaders()
		for key, value := range headers {
			req.Header.Set(key, value)
		}

		client := &http.Client{Timeout: 300 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("upload part %d failed: %w", i, err)
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return fmt.Errorf("upload part %d failed with status %d: %s", i, resp.StatusCode, string(body))
		}

		etag := resp.Header.Get("Etag")
		b.etags = append(b.etags, etag)
		fmt.Printf("分片%d上传成功: %s\n", i, etag)
		resp.Body.Close()
	}

	return nil
}

// commitUpload 提交上传数据
func (b *BcutASR) commitUpload() error {
	payload := map[string]interface{}{
		"InBossKey":  b.inBossKey,
		"ResourceId": b.resourceID,
		"Etags":      fmt.Sprintf("%s", b.etags[0]), // 简化处理，实际应该是逗号分隔的所有etags
		"UploadId":   b.uploadID,
		"model_id":   "8",
	}

	// 正确处理多个etags
	if len(b.etags) > 1 {
		etagsStr := ""
		for i, etag := range b.etags {
			if i > 0 {
				etagsStr += ","
			}
			etagsStr += etag
		}
		payload["Etags"] = etagsStr
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", APICommitUpload, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	headers := b.buildHeaders()
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("commit upload failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("failed to parse JSON response: %w", err)
	}

	data, ok := result["data"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid response, no data field: %s", string(body))
	}

	b.downloadURL = data["download_url"].(string)
	fmt.Println("提交成功")

	return nil
}

// Upload 执行完整的上传流程
func (b *BcutASR) Upload() error {
	if err := b.requestUpload(); err != nil {
		return fmt.Errorf("request upload failed: %w", err)
	}

	if err := b.uploadParts(); err != nil {
		return fmt.Errorf("upload parts failed: %w", err)
	}

	if err := b.commitUpload(); err != nil {
		return fmt.Errorf("commit upload failed: %w", err)
	}

	return nil
}

// CreateTask 创建转换任务
func (b *BcutASR) CreateTask() error {
	payload := map[string]interface{}{
		"resource": b.downloadURL,
		"model_id": "8",
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", APICreateTask, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	headers := b.buildHeaders()
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("create task failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("failed to parse JSON response: %w", err)
	}

	data, ok := result["data"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid response, no data field: %s", string(body))
	}

	b.taskID = data["task_id"].(string)
	fmt.Printf("任务已创建: %s\n", b.taskID)

	return nil
}

// QueryResult 查询转换结果
func (b *BcutASR) QueryResult() (map[string]interface{}, error) {
	url := fmt.Sprintf("%s?model_id=7&task_id=%s", APIQueryResult, b.taskID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	headers := b.buildHeaders()
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("query result failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse JSON response: %w", err)
	}

	data, ok := result["data"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid response, no data field: %s", string(body))
	}

	return data, nil
}

// Run 执行完整的ASR工作流
func (b *BcutASR) Run() (*ASRResult, error) {
	// 上传文件
	if err := b.Upload(); err != nil {
		return nil, fmt.Errorf("upload failed: %w", err)
	}

	// 创建任务
	if err := b.CreateTask(); err != nil {
		return nil, fmt.Errorf("create task failed: %w", err)
	}

	// 轮询查询结果
	maxRetries := 500
	for i := 0; i < maxRetries; i++ {
		time.Sleep(1 * time.Second)

		taskData, err := b.QueryResult()
		if err != nil {
			return nil, fmt.Errorf("query failed: %w", err)
		}

		state := int(taskData["state"].(float64))

		// state: 4表示完成
		if state == 4 {
			fmt.Println("转换成功")
			resultStr := taskData["result"].(string)

			var resultData map[string]interface{}
			if err := json.Unmarshal([]byte(resultStr), &resultData); err != nil {
				return nil, fmt.Errorf("failed to parse result JSON: %w", err)
			}

			return b.makeSegments(resultData), nil
		} else if state == 3 {
			return nil, errors.New("ASR task failed")
		}
	}

	return nil, errors.New("ASR task timeout")
}

// makeSegments 解析识别结果
func (b *BcutASR) makeSegments(resultData map[string]interface{}) *ASRResult {
	utterances, ok := resultData["utterances"].([]interface{})
	if !ok {
		return &ASRResult{
			Segments: []ASRSegment{},
			RawData:  resultData,
		}
	}

	var segments []ASRSegment

	for _, u := range utterances {
		utterance := u.(map[string]interface{})
		segments = append(segments, ASRSegment{
			Text:      utterance["transcript"].(string),
			StartTime: int64(utterance["start_time"].(float64)),
			EndTime:   int64(utterance["end_time"].(float64)),
		})
	}

	return &ASRResult{
		Segments: segments,
		RawData:  resultData,
	}
}
