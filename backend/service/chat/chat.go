package chat

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// Reply 是一次模型流式回复。Next 依次返回终端里会 print 出来的正文片段。
type Reply struct {
	body   io.ReadCloser
	scan   *bufio.Scanner
	cancel context.CancelFunc
}

// Open 按 test.py 的方式请求本地模型：stream=true，只取 delta.content。
func Open(parent context.Context, message string) (*Reply, error) {
	loadEnv()
	base := strings.TrimRight(getenv("LLM_BASE_URL", "http://127.0.0.1:8080/v1"), "/")
	model := getenv("LLM_MODEL", "local-model")
	timeout := time.Duration(intEnv("LLM_READ_TIMEOUT_SECONDS", 600)) * time.Second
	ctx, cancel := context.WithTimeout(parent, timeout)

	payload, err := json.Marshal(map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": message},
		},
		"stream": true,
	})
	if err != nil {
		cancel()
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		cancel()
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if key := os.Getenv("LLM_API_KEY"); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		return nil, err
	}
	if resp.StatusCode >= http.StatusBadRequest {
		resp.Body.Close()
		cancel()
		return nil, io.ErrUnexpectedEOF
	}
	scan := bufio.NewScanner(resp.Body)
	scan.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	return &Reply{body: resp.Body, scan: scan, cancel: cancel}, nil
}

// Next 返回下一段正文。流结束时返回 io.EOF。
func (r *Reply) Next() (string, error) {
	for r.scan.Scan() {
		text := contentOf(r.scan.Text())
		if text == "" {
			continue
		}
		return text, nil
	}
	if err := r.scan.Err(); err != nil {
		return "", err
	}
	return "", io.EOF
}

// Close 结束上游连接。
func (r *Reply) Close() {
	r.body.Close()
	r.cancel()
}

func contentOf(line string) string {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "data:") {
		return ""
	}
	raw := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
	if raw == "" || raw == "[DONE]" {
		return ""
	}
	var chunk struct {
		Choices []struct {
			Delta struct {
				Content any `json:"content"`
			} `json:"delta"`
		} `json:"choices"`
	}
	if err := json.Unmarshal([]byte(raw), &chunk); err != nil || len(chunk.Choices) == 0 {
		return ""
	}
	text, _ := chunk.Choices[0].Delta.Content.(string)
	return text
}

func loadEnv() {
	for _, path := range []string{".env", "../.env"} {
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(raw), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
				continue
			}
			key, value, _ := strings.Cut(line, "=")
			key = strings.TrimSpace(key)
			value = strings.Trim(strings.TrimSpace(value), `"'`)
			if _, exists := os.LookupEnv(key); exists {
				continue
			}
			_ = os.Setenv(key, value)
		}
	}
}

func getenv(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func intEnv(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}
