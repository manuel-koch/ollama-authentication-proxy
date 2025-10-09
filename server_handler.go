package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/ollama/ollama/api"
)

type PreloadModelStatus int64

const (
	Unknown PreloadModelStatus = iota
	InProgress
	Preloaded
)

type ServerHandler struct {
	apiKeys []string

	preloadModels      []string
	preloadModelStatus PreloadModelStatus

	upstreamBaseURL               *url.URL
	lastSuccessfulPingTime        time.Time
	userModelMetricsWebhookUrl    string
	userModelMetricsWebhookApiKey string
}

// NewServerHandler will create a new server
func NewServerHandler(apiKeys []string, preloadModels []string) *ServerHandler {
	return &ServerHandler{
		apiKeys:       apiKeys,
		preloadModels: preloadModels,
	}
}

// SetUpstreamURL will set base url of upstream on server
func (s *ServerHandler) SetUpstreamURL(baseURL *url.URL) {
	s.upstreamBaseURL = baseURL
	slog.Info(fmt.Sprintf("Upstream at %s", baseURL))
}

// GetUpstreamURL will return the URL of the upstream
func (s *ServerHandler) GetUpstreamURL() *url.URL {
	return s.upstreamBaseURL
}

// SetUpstreamURL will set base url of upstream on server
func (s *ServerHandler) SetUserModelMetricsWebhook(url string, apiKey string) {
	s.userModelMetricsWebhookUrl = url
	s.userModelMetricsWebhookApiKey = apiKey

	apiKeyUsage := "w/o API key"
	if len(s.userModelMetricsWebhookApiKey) > 0 {
		apiKeyUsage = "w/ API key"
	}
	slog.Info(fmt.Sprintf("User model metrics webhook at %s %s", s.userModelMetricsWebhookUrl, apiKeyUsage))
}

// ServeHttpProxy will be called by the http server to handle a request that should be proxied to backend
func (s *ServerHandler) ServeHttpProxy(w http.ResponseWriter, r *http.Request) {
	backendURL := s.GetUpstreamURL()
	requestId := uuid.New().String()
	logger := slog.With(
		"requestId", requestId,
		"client", r.RemoteAddr,
		"backendURL", backendURL,
		"method", r.Method,
		"url", r.URL,
		"proto", r.Proto)
	logger.Info("Handle request")
	if s.authRequestHandle(w, r) {
		upstreamHandler := NewProxyHandler(backendURL, s.forwardUserModelMetrics, logger)
		upstreamHandler.ProxyRequest(w, r)
	}
}

// ServeHttpPing will be called by the http server to handle a "ping" request, checking if upstream is running ok
func (s *ServerHandler) ServeHttpPing(w http.ResponseWriter, r *http.Request) {
	backendURL := s.GetUpstreamURL()
	requestId := uuid.New().String()
	logger := slog.With(
		"requestId", requestId,
		"client", r.RemoteAddr,
		"backendURL", backendURL,
		"method", r.Method,
		"url", r.URL,
		"proto", r.Proto)
	if s.authRequestHandle(w, r) {
		if s.isUpstreamRunning() {
			switch s.preloadModelStatus {
			case Unknown:
				logger.Warn("Upstream available, preloading unknown")
				w.WriteHeader(http.StatusNoContent)
				w.Write([]byte("{\"status\": \"Model preload not started yet\"}"))
			case InProgress:
				logger.Info("Upstream is available, preloading in progress")
				w.WriteHeader(http.StatusNoContent)
				w.Write([]byte("{\"status\": \"Model preload in progress\"}"))
			case Preloaded:
				logFunc := logger.Debug
				if time.Since(s.lastSuccessfulPingTime) > 60*time.Second {
					logFunc = logger.Info
				}
				logFunc("Upstream is available, preloading done")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("{\"status\": \"Models preloaded\"}"))
			}
		} else {
			logger.Warn("Upstream is unavailable")
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("{}"))
		}
	} else {
		logger.Error("Unauthorized ping")
	}
}

// authRequestHandler checks request for authorization details and
// returns true when request is authorized.
func (s *ServerHandler) authRequestHandle(w http.ResponseWriter, r *http.Request) bool {
	if s.requireApiKeyAuthorization() {
		authHeader := r.Header.Get("Authorization")

		if authHeader == "" {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintln(w, "Unauthorized: Missing Authorization header")
			slog.Info("Unauthorized: Missing Authorization header")
			return false
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintln(w, "Unauthorized: Invalid Authorization header format")
			slog.Info("Unauthorized: Invalid Authorization header format")
			return false
		}

		apiKey := parts[1]

		if !s.isValidAPIKey(apiKey) {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintln(w, "Unauthorized: Invalid API key")
			slog.Info("Unauthorized: Invalid API key")
			return false
		}
	}

	r.Header.Del("Authorization")

	return true
}

func (s *ServerHandler) isUpstreamRunning() bool {
	if s.upstreamBaseURL == nil {
		slog.Error("Failed to ping unknown upstream")
		return false
	}
	resp, err := http.Get(s.upstreamBaseURL.String())
	if err != nil {
		slog.Error("Failed to ping upstream", "error", err)
		return false
	}
	isSuccessStatus := resp.StatusCode/100 == 2
	if !isSuccessStatus {
		slog.Error("Failed to ping upstream", "status", resp.StatusCode)
	} else {
		s.lastSuccessfulPingTime = time.Now()
	}

	return isSuccessStatus
}

func (s *ServerHandler) PreLoadModels(ctx context.Context) {
	if s.upstreamBaseURL == nil {
		slog.Error("Failed to preload models: unknown upstream")
		return
	}
	for {
		slog.Info("Waiting for upstream to be running...")
		if s.isUpstreamRunning() {
			break
		}
		time.Sleep(2 * time.Second)
	}

	s.preloadModelStatus = InProgress

	slog.Info(fmt.Sprintf("Preloading %d models...", len(s.preloadModels)))
	client := api.NewClient(s.upstreamBaseURL, http.DefaultClient)
	for _, model := range s.preloadModels {
		pullRequest := &api.PullRequest{
			Model: model,
		}
		var lastTotal int64 = -1
		var lastProgressPercentage int64 = -1
		progressFunc := func(resp api.ProgressResponse) error {
			if resp.Total != lastTotal {
				lastTotal = resp.Total
			}
			if resp.Completed > 0 {
				progressPercentage := int64(float64(resp.Completed) / float64(lastTotal) * 100)
				if progressPercentage != lastProgressPercentage {
					lastProgressPercentage = progressPercentage
					slog.Info("Loading", "model", model, "status", resp.Status, "progressPercentage", fmt.Sprintf("%d%%", progressPercentage))
				}
			}
			return nil
		}
		slog.Info(fmt.Sprintf("Loading model %s...", model))
		err := client.Pull(ctx, pullRequest, progressFunc)
		if err != nil {
			slog.Error("Failed to pull", "model", model, "error", err)
		} else {
			slog.Info(fmt.Sprintf("Loaded model %s", model))
		}
	}

	s.preloadModelStatus = Preloaded
	slog.Info(fmt.Sprintf("Preloaded %d models", len(s.preloadModels)))

	listResponse, err := client.List(ctx)
	if err != nil {
		slog.Error("Failed to list current models", "error", err)
	} else {
		var totalSize int64 = 0
		for _, model := range listResponse.Models {
			totalSize += model.Size
			sizeGB := float64(model.Size) / 1024.0 / 1024.0 / 1024.0
			slog.Info(fmt.Sprintf("Found model %s, size %0.1fGB", model.Name, sizeGB))
		}
		totalSizeGB := float64(totalSize) / 1024.0 / 1024.0 / 1024.0
		slog.Info(fmt.Sprintf("Found %d loaded models with total size %0.1fGB", len(listResponse.Models), totalSizeGB))
	}
}

// forwardUserModelMetrics forwards the give ollama usage metrics to selected webhook.
func (s *ServerHandler) forwardUserModelMetrics(userModelMetrics UserModelMetrics) {
	if len(s.userModelMetricsWebhookUrl) == 0 {
		slog.Debug("Skip forwarding user model metrics: no webhook url")
		return
	}

	buf, err := json.Marshal(userModelMetrics)
	if err != nil {
		slog.Error("Failed to forward user model metrics: marshal failed", "webhook", s.userModelMetricsWebhookUrl, "error", err)
		return
	}

	req, err := http.NewRequest("POST", s.userModelMetricsWebhookUrl, bytes.NewReader(buf))
	if err != nil {
		slog.Error("Failed to forward user model metrics: request failed", "webhook", s.userModelMetricsWebhookUrl, "error", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	if len(s.userModelMetricsWebhookApiKey) > 0 {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", s.userModelMetricsWebhookApiKey))
	}

	response, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Error("Failed to forward user model metrics: POST failed", "webhook", s.userModelMetricsWebhookUrl, "error", err)
	} else if (response.StatusCode / 100) != 2 {
		slog.Error("Failed to forward user model metrics: POST indicates failure", "webhook", s.userModelMetricsWebhookUrl, "status", response.StatusCode)
	} else {
		slog.Info("Forwarded user model metrics to", "webhook", s.userModelMetricsWebhookUrl)
	}
	if response != nil {
		response.Body.Close()
	}
}

// requireApiKeyAuthorization checks if authentication with API key is required.
func (s *ServerHandler) requireApiKeyAuthorization() bool {
	return len(s.apiKeys) > 0
}

// isValidAPIKey checks if the provided API key is valid.
func (s *ServerHandler) isValidAPIKey(apiKey string) bool {
	return slices.Contains(s.apiKeys, strings.TrimSpace(apiKey))
}
