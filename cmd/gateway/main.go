package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/example/llm-optimization-gateway/internal/gateway"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	srv := gateway.NewServer(port)

	// Channel to listen for OS signals for graceful shutdown.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Start server in a goroutine.
	go func() {
		fmt.Printf("Starting gateway server on :%s\n", port)
		if err := srv.Start(); err != nil {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	// Block until a signal is received.
	sig := <-quit
	fmt.Printf("\nReceived signal %s, shutting down gracefully...\n", sig)

	if err := srv.Shutdown(context.Background()); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	fmt.Println("Server exited cleanly")
}
