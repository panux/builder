package srvctx

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

// Context is a global context that is cancelled by a SIGTERM.
var Context context.Context

// Wait is a server-wide sync.WaitGroup.
// Server shuts down when done.
var Wait sync.WaitGroup

// Cancel is the global server cancellation function.
var Cancel context.CancelFunc

// Setup Context.
func init() {
	Context, Cancel = context.WithCancel(context.Background())
	sigch := make(chan os.Signal, 1)
	Wait.Add(1)
	go func() {
		defer Wait.Done()
		select {
		case <-sigch:
			log.Println("Initiating shutdown")
			Cancel()
		case <-Context.Done():
		}
	}()
	signal.Notify(sigch, syscall.SIGTERM)
}

// HTTP shuts down the HTTP server on server shutdown (async).
func HTTP(srv *http.Server) {
	Wait.Add(1)
	go func() { // run http server shutdown
		defer Wait.Done()
		<-Context.Done()
		err := srv.Shutdown(context.Background())
		if err != nil {
			log.Printf("HTTP shutdown error: %q\n", err.Error())
		}
	}()
}
