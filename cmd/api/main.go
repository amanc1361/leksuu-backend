package main

import (
	"log"

	"github.com/leksuu/platform/internal/platform/app"
	"github.com/leksuu/platform/internal/platform/config"
)

func main() {
	application, err := app.New(config.Load())
	if err != nil {
		log.Fatalf("startup: %v", err)
	}
	if err := application.Run(); err != nil {
		log.Fatalf("run: %v", err)
	}
}
