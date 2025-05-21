// A helper tool to be used by nginx to authenticate incoming request
// via "Bearer" token/apikey before the traffic gets forwarded to ollama.

package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/url"
	"os"
	"os/signal"
	"path"
	"strconv"
	"strings"
	"syscall"
)

var logger *log.Logger

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
	return host
}

// getPort returns the port to bind to
func getPort() int {
	var port = 18434
	if envPort, found := os.LookupEnv("AUTHORIZATION_PORT"); found {
		if p, err := strconv.Atoi(envPort); err == nil && p != 0 {
			port = p
		}
	}
	return port
}

// getOllamaHost returns the hostname to bind to
func getOllamaHost() string {
	var host = "localhost:11434"
	if envHost, found := os.LookupEnv("OLLAMA_HOST"); found {
		host = strings.TrimSpace(envHost)
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
	var apiKeys = getApiKeys()

	backendURL, err := url.Parse(fmt.Sprintf("http://%s", getOllamaHost()))
	if err != nil {
		log.Fatal(err)
	}

	serverHandler := NewServerHandler(apiKeys)
	serverHandler.AddBackend(backendURL)

	ctx, cancelCtx := context.WithCancel(context.Background())
	server := NewServer(ctx, host, port, serverHandler.ServeHTTP)
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

	// block until we receive the "done" via a channel
	<-done
	cancelCtx()
	if shutdownErr := server.Shutdown(ctx); shutdownErr != nil {
		slog.Error("Failed to shutdown server", "error", shutdownErr)
		return
	}
	slog.Info("Done.")
}
