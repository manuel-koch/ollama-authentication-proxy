package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
)

type Server struct {
	http.Server
}

func NewServer(ctx context.Context, host string, port int, handlerFuncs map[string]func(http.ResponseWriter, *http.Request)) *Server {
	addr := fmt.Sprintf("%s:%d", host, port)
	mux := http.NewServeMux()
	for pattern, handler := range handlerFuncs {
		mux.HandleFunc(pattern, handler)
	}
	return &Server{
		http.Server{
			Addr:    addr,
			Handler: mux,
			BaseContext: func(l net.Listener) context.Context {
				// use a new context with additional variable available in the context
				// under a given key.
				//ctx = context.WithValue(ctx, "the_key", l.Addr().String())
				return ctx
			},
		},
	}
}

func (s *Server) Run() {
	slog.Info(fmt.Sprintf("Server listening at %s", s.Addr))
	serverErr := s.ListenAndServe()
	if !errors.Is(serverErr, http.ErrServerClosed) {
		slog.Error("Failed to start server", "error", serverErr)
	} else {
		slog.Info("Server stopped")
	}
}
