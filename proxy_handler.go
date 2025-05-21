package main

import (
	"bufio"
	"io"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
)

type ProxyHandler struct {
	Proxy *httputil.ReverseProxy
}

func (t *ProxyHandler) RoundTrip(request *http.Request) (*http.Response, error) {
	return http.DefaultTransport.RoundTrip(request)
}

func (h *ProxyHandler) ProxyRequest(w http.ResponseWriter, r *http.Request) {
	h.Proxy.ServeHTTP(w, r)
}

func NewProxyHandler(upstreamURL *url.URL, logger *slog.Logger) *ProxyHandler {
	ph := &ProxyHandler{
		Proxy: &httputil.ReverseProxy{
			Rewrite: func(r *httputil.ProxyRequest) {
				r.SetXForwarded()
				r.SetURL(upstreamURL)
			},
			ModifyResponse: func(response *http.Response) error {
				logger.Info("Got backend response", "status", response.StatusCode)
				pr, pw := io.Pipe()
				body := response.Body
				response.Body = pr
				go func() {
					defer pw.Close()
					totalSize := 0
					reader := bufio.NewReader(body)
					for {
						chunk, lineErr := reader.ReadBytes('\n')
						chunkSize := len(chunk)
						totalSize += chunkSize
						logger.Debug("Got", "chunk", chunk)
						if _, err := pw.Write(chunk); err != nil {
							logger.Info("Failed to write", "error", err)
						}
						if lineErr == io.EOF {
							logger.Info("Done backend response", "bodySize", totalSize)
							return
						}
						if lineErr != nil {
							logger.Error("Failed backend response", "error", lineErr, "bodySize", totalSize)
							return
						}
					}
				}()
				return nil
			},
		},
	}

	ph.Proxy.Transport = ph
	return ph
}
