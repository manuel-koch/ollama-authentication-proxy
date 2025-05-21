package main

import (
	"fmt"
	"github.com/google/uuid"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
)

type ServerHandler struct {
	apiKeys          []string
	upstreamBaseURLs []*url.URL
}

// NewServerHandler will create a new server
func NewServerHandler(apiKeys []string) *ServerHandler {
	return &ServerHandler{
		apiKeys:          apiKeys,
		upstreamBaseURLs: make([]*url.URL, 0),
	}
}

// AddBackend will add given backend to server
func (s *ServerHandler) AddBackend(baseURL *url.URL) {
	s.upstreamBaseURLs = append(s.upstreamBaseURLs, baseURL)
	slog.Info(fmt.Sprintf("Added backend at %s", baseURL))
}

// GetBackendURL will return the URL of a backend instance
func (s *ServerHandler) GetBackendURL() *url.URL {
	return s.upstreamBaseURLs[0]
}

// ServeHTTP will be called by the http server to handle a request
func (s *ServerHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	backendURL := s.GetBackendURL()
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
		upstreamHandler := NewProxyHandler(backendURL, logger)
		upstreamHandler.ProxyRequest(w, r)
	}
}

// authRequestHandler checks request for authorization details and
// returns true when request is authorized.
func (s *ServerHandler) authRequestHandle(w http.ResponseWriter, r *http.Request) bool {
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

	r.Header.Del("Authorization")

	return true
}

// isValidAPIKey checks if the provided API key is valid.
func (s *ServerHandler) isValidAPIKey(apiKey string) bool {
	return slices.Contains(s.apiKeys, strings.TrimSpace(apiKey))
}
