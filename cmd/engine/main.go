package main

import (
	"os"

	"github.com/joho/godotenv"

	"github.com/Raoon-Tech/bpmn-ai/internal/cli"
)

func main() {
	// Load .env file if it exists (ignores error if missing).
	_ = godotenv.Load()
	os.Exit(cli.Run(os.Args[1:]))
}
