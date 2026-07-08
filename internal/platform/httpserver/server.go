// Package httpserver is a thin, reusable lifecycle wrapper around net/http.Server.
package httpserver

import (
	"context"
	"net/http"
	"time"
)

type Server struct{ srv *http.Server }

func New(addr string, handler http.Handler) *Server {
	return &Server{srv: &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}}
}

func (s *Server) Start() error                       { return s.srv.ListenAndServe() }
func (s *Server) Shutdown(ctx context.Context) error { return s.srv.Shutdown(ctx) }
