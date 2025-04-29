// A helper tool to be used by nginx to authenticate incoming request
// via "Bearer" token/apikey before the traffic gets forwarded to ollama.

package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"
)

var logger *log.Logger
var apiKeys []string = make([]string, 0)

// authRequestHandler handles the authentication request from Nginx.
func authRequestHandle(w http.ResponseWriter, r *http.Request) {
	authHeader := r.Header.Get("Authorization")

	if authHeader == "" {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintln(w, "Unauthorized: Missing Authorization header")
		logger.Println("Unauthorized: Missing Authorization header")
		return
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintln(w, "Unauthorized: Invalid Authorization header format")
		logger.Println("Unauthorized: Invalid Authorization header format")
		return
	}

	apiKey := parts[1]

	if isValidAPIKey(apiKey) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "OK") // Success status
	} else {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintln(w, "Unauthorized: Invalid API key")
		logger.Println("Unauthorized: Invalid API key")
	}
}

// isValidAPIKey checks if the provided API key is valid.
func isValidAPIKey(apiKey string) bool {
	return slices.Contains(apiKeys, strings.TrimSpace(apiKey))
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
	var port = 8080
	if envPort, found := os.LookupEnv("AUTHORIZATION_PORT"); found {
		if p, err := strconv.Atoi(envPort); err == nil && p != 0 {
			port = p
		}
	}
	return port
}

// getAuthPath returns the path of the authentication endpoint
func getAuthPath() string {
	var authPath = "/auth"
	if envPath, found := os.LookupEnv("AUTHORIZATION_PATH"); found {
		authPath = strings.TrimSpace(envPath)
	}
	return authPath
}

// getApiKeys extracts API keys from environment variable(s)
func getApiKeys() {
	for _, envVar := range os.Environ() {
		if strings.HasPrefix(envVar, "AUTHORIZATION_APIKEY") {
			apiKey := strings.TrimSpace(strings.SplitN(envVar, "=", 2)[1])
			if len(apiKey) > 0 {
				apiKeys = append(apiKeys, apiKey)
			}
		}
	}
	logger.Printf("Using %d API keys.\n", len(apiKeys))
}

func createLogger() *log.Logger {
	var prefix = ""
	if execPath, err := os.Executable(); err == nil {
		prefix = fmt.Sprintf("[%s] ", path.Base(execPath))
	}
	return log.New(os.Stdout, prefix, log.LstdFlags|log.LUTC)
}

func main() {
	logger = createLogger()
	var authPath = getAuthPath()
	var host = getHost()
	var port = getPort()
	getApiKeys()

	http.HandleFunc(authPath, authRequestHandle)
	logger.Printf("Authenticating server listening on %s:%d%s\n", host, port, authPath)
	logger.Fatal(http.ListenAndServe(fmt.Sprintf("%s:%d", host, port), nil))
}
