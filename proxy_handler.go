package main

import (
	"bufio"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/ollama/ollama/api"
)

type UserModelMetrics struct {
	CreatedAt time.Time `json:"created_at"`
	Model     string    `json:"model"`
	UserId    string    `json:"user_id,omitempty"`
	UserName  string    `json:"user_name,omitempty"`
	api.Metrics
}

type ProxyHandler struct {
	Proxy *httputil.ReverseProxy

	logger                   *slog.Logger
	upstreamURL              *url.URL
	userModelMetricsCallback func(userModelMetrics UserModelMetrics)
	userId                   string
	userName                 string
}

func (t *ProxyHandler) RoundTrip(request *http.Request) (*http.Response, error) {
	return http.DefaultTransport.RoundTrip(request)
}

func (h *ProxyHandler) ProxyRequest(w http.ResponseWriter, r *http.Request) {
	h.Proxy.ServeHTTP(w, r)
}

func (h *ProxyHandler) rewrite(r *httputil.ProxyRequest) {
	r.SetXForwarded()
	r.SetURL(h.upstreamURL)

	for key := range r.In.Header {
		lowerKey := strings.ToLower(key)
		if strings.HasSuffix(lowerKey, "-user-id") {
			h.userId = strings.TrimSpace(r.In.Header.Get(key))
		} else if strings.HasSuffix(lowerKey, "-user-name") {
			h.userName = strings.TrimSpace(r.In.Header.Get(key))
		}
	}
}

func (h *ProxyHandler) modifyResponse(response *http.Response) error {
	h.logger.Info("Got backend response", "status", response.StatusCode)
	pr, pw := io.Pipe()
	body := response.Body
	response.Body = pr
	go func() {
		defer pw.Close()
		totalSize := 0
		reader := bufio.NewReader(body)
		for {
			chunk, lineErr := reader.ReadBytes('\n')
			h.logger.Debug("Got", "chunk", chunk)
			chunkSize := len(chunk)
			totalSize += chunkSize
			if _, err := pw.Write(chunk); err != nil {
				h.logger.Info("Failed to write", "error", err)
			}
			if lineErr == io.EOF {
				h.logger.Info("Done backend response", "bodySize", totalSize)
				return
			}
			if lineErr != nil {
				h.logger.Error("Failed backend response", "error", lineErr, "bodySize", totalSize)
				return
			}
			if h.userModelMetricsCallback != nil {
				chatResponse := extractDoneChatResponse(chunk)
				if chatResponse != nil {
					userModelMetrics := UserModelMetrics{
						CreatedAt: chatResponse.CreatedAt,
						Model:     chatResponse.Model,
						UserId:    h.userId,
						UserName:  h.userName,
						Metrics:   chatResponse.Metrics,
					}
					go h.userModelMetricsCallback(userModelMetrics)
				}
			}
		}
	}()
	return nil
}

func NewProxyHandler(upstreamURL *url.URL, userModelMetricsCallback func(metrics UserModelMetrics), logger *slog.Logger) *ProxyHandler {
	ph := &ProxyHandler{
		Proxy: &httputil.ReverseProxy{},
	}
	ph.Proxy.Transport = ph
	ph.Proxy.Rewrite = ph.rewrite
	ph.Proxy.ModifyResponse = ph.modifyResponse
	ph.logger = logger
	ph.upstreamURL = upstreamURL
	ph.userModelMetricsCallback = userModelMetricsCallback
	return ph
}

func extractDoneChatResponse(data []byte) *api.ChatResponse {
	chatResponse := api.ChatResponse{}

	if err := json.Unmarshal(data, &chatResponse); err != nil {
		slog.Error("Failed to extract chat response", "error", err)
		return nil
	} else {
		if chatResponse.Done {
			return &chatResponse
		}
		return nil
	}
}
