// A reverse-proxy written in golang that authenticates incoming request
// via "Bearer" token/apikey before the traffic gets forwarded to ollama.
// It will trigger loading selected ollama models on startup.
// It provides a "/ping" endpoint to health-check ollama.

package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path"
	"strconv"
	"strings"
	"syscall"
)

// getLogLevel returns a log level
func getLogLevel() slog.Level {
	if envLevel, found := os.LookupEnv("AUTHORIZATION_LOG_LEVEL"); found {
		if strings.ToLower(envLevel) == "error" {
			return slog.LevelError
		}
		if strings.ToLower(envLevel) == "warn" || strings.ToLower(envLevel) == "warning" {
			return slog.LevelWarn
		}
		if strings.ToLower(envLevel) == "info" {
			return slog.LevelInfo
		}
		if strings.ToLower(envLevel) == "debug" {
			return slog.LevelDebug
		}
	}
	return slog.LevelInfo
}

// getLogJson returns whether to log in JSON format
func getLogJson() bool {
	if envBool, found := os.LookupEnv("AUTHORIZATION_LOG_JSON"); found {
		if strings.ToLower(envBool) == "true" {
			return true
		}
	}
	return false
}

// getHost returns the hostname to bind to
func getHost() string {
	var host = "0.0.0.0"
	if envHost, found := os.LookupEnv("AUTHORIZATION_HOST"); found {
		host = strings.TrimSpace(envHost)
	}
	if envHost, found := os.LookupEnv("HOST"); found {
		host = strings.TrimSpace(envHost)
	}
	return host
}

// getPort returns the port to bind to
func getPort() int {
	var port = 80
	if envPort, found := os.LookupEnv("AUTHORIZATION_PORT"); found {
		if p, err := strconv.Atoi(envPort); err == nil && p != 0 {
			port = p
		}
	}
	if envPort, found := os.LookupEnv("PORT"); found {
		if p, err := strconv.Atoi(envPort); err == nil && p != 0 {
			port = p
		}
	}
	return port
}

// getPortHealth returns the port to bind "/ping" endpoint to
func getPortHealth() int {
	var port = 80
	if envPort, found := os.LookupEnv("PORT_HEALTH"); found {
		if p, err := strconv.Atoi(envPort); err == nil && p != 0 {
			port = p
		}
	}
	return port
}

// getOllamaHostPort returns the hostname and port of running ollama instance
func getOllamaHostPort() string {
	var host = "localhost:11434"
	if envHost, found := os.LookupEnv("OLLAMA_HOST"); found {
		if strings.Contains(envHost, ":") {
			host = strings.TrimSpace(envHost)
		} else {
			slog.Debug("OLLAMA_HOST environment variable doesn't contain a port, using default host")
		}
	} else {
		slog.Debug("OLLAMA_HOST environment variable not set, using default host")
	}
	return host
}

// getApiKeys extracts API keys from environment variable(s)
func getApiKeys() []string {
	apiKeys := make([]string, 0)
	for _, envVar := range os.Environ() {
		if strings.HasPrefix(envVar, "AUTHORIZATION_APIKEY") {
			apiKey := strings.TrimSpace(strings.SplitN(envVar, "=", 2)[1])
			if len(apiKey) > 0 {
				apiKeys = append(apiKeys, apiKey)
			}
		}
	}
	slog.Info(fmt.Sprintf("Using %d API keys", len(apiKeys)))
	return apiKeys
}

// getPreloadModels extracts models names to be pre-loaded on startup from environment variable(s)
func getPreloadModels() []string {
	models := make([]string, 0)
	for _, envVar := range os.Environ() {
		if strings.HasPrefix(envVar, "PRELOAD_MODEL") {
			model := strings.TrimSpace(strings.SplitN(envVar, "=", 2)[1])
			if len(model) > 0 {
				models = append(models, model)
			}
		}
	}
	slog.Info(fmt.Sprintf("Using %d preload models", len(models)))
	return models
}

// getUserModelMetricsWebhookUrl returns the URL of a webhook that will receive user model metrics
func getUserModelMetricsWebhookUrl() string {
	var url = ""
	if envUrl, found := os.LookupEnv("USER_MODEL_METRICS_WEBHOOK_URL"); found {
		url = strings.TrimSpace(envUrl)
	}
	if len(url) > 0 {
		slog.Info(fmt.Sprintf("Using user model metrics webhook url %s", url))
	}
	return url
}

// getUserModelMetricsWebhookUrl returns the URL of a webhook that will receive user model metrics
func getUserModelMetricsWebhookApiKey() string {
	var apiKey = ""
	if envApiKey, found := os.LookupEnv("USER_MODEL_METRICS_WEBHOOK_API_KEY"); found {
		apiKey = strings.TrimSpace(envApiKey)
	}
	if len(apiKey) > 0 {
		slog.Info("Using user model metrics webhook API key")
	}
	return apiKey
}

func initLogging(level slog.Level, logJson bool) {
	var logger *slog.Logger
	logOptions := &slog.HandlerOptions{
		AddSource: true,
		Level:     level,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// Use filename and line of the source
			if a.Key == slog.SourceKey {
				s := a.Value.Any().(*slog.Source)
				s.File = path.Base(s.File)
			}
			return a
		},
	}
	if logJson {
		logger = slog.New(slog.NewJSONHandler(os.Stdout, logOptions))
	} else {
		logger = slog.New(slog.NewTextHandler(os.Stdout, logOptions))
	}
	slog.SetDefault(logger)
}

func main() {
	initLogging(getLogLevel(), getLogJson())

	var host = getHost()
	var port = getPort()
	var portHealth = getPortHealth()
	var apiKeys = getApiKeys()
	var preloadModels = getPreloadModels()
	var userModelMetricsWebhookUrl = getUserModelMetricsWebhookUrl()
	var userModelMetricsWebhookApiKey = getUserModelMetricsWebhookApiKey()

	ctx, cancelCtx := context.WithCancel(context.Background())

	backendURL, err := url.Parse(fmt.Sprintf("http://%s", getOllamaHostPort()))
	if err != nil {
		log.Fatal(err)
	}

	serverHandler := NewServerHandler(apiKeys, preloadModels)
	serverHandler.SetUpstreamURL(backendURL)
	serverHandler.SetUserModelMetricsWebhook(userModelMetricsWebhookUrl, userModelMetricsWebhookApiKey)

	serverHandlerFuncs := make(map[string]func(http.ResponseWriter, *http.Request))
	serverHandlerFuncs["/"] = serverHandler.ServeHttpProxy

	var serverPing *Server = nil
	if port != portHealth {
		pingFuncs := make(map[string]func(http.ResponseWriter, *http.Request))
		pingFuncs["/ping"] = serverHandler.ServeHttpPing
		serverPing = NewServer(ctx, host, portHealth, pingFuncs)
		go serverPing.Run()
		pingUrl := fmt.Sprintf("http://%s", serverPing.Addr)
		slog.Info(fmt.Sprintf("Ping listening at %s", pingUrl))
	} else {
		serverHandlerFuncs["/ping"] = serverHandler.ServeHttpPing
	}

	server := NewServer(ctx, host, port, serverHandlerFuncs)
	go server.Run()
	serverUrl := fmt.Sprintf("http://%s", server.Addr)
	slog.Info(fmt.Sprintf("Authenticating proxy listening at %s", serverUrl))

	sigs := make(chan os.Signal, 1)
	done := make(chan bool, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigs
		slog.Info("Received", "signal", sig)
		done <- true
	}()

	go serverHandler.PreLoadModels(ctx)

	// block until we receive the "done" via channel
	<-done
	cancelCtx()

	if serverPing != nil {
		slog.Info("Shutdown ping server")
		if shutdownErr := serverPing.Shutdown(context.Background()); shutdownErr != nil {
			slog.Error("Failed to shutdown ping server", "error", shutdownErr)
			return
		}
	}
	slog.Info("Shutdown server")
	if shutdownErr := server.Shutdown(context.Background()); shutdownErr != nil {
		slog.Error("Failed to shutdown server", "error", shutdownErr)
		return
	}
	slog.Info("Done.")
}
