package utils

import (
	"log"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
)

// LoadEnv finds and loads a .env file by walking up from the working
// directory (max 5 levels).  It is a no-op if no .env is found so that
// production deployments that inject real env vars are unaffected.
func LoadEnv() {
	dir, err := os.Getwd()
	if err != nil {
		return
	}
	for range 5 {
		path := filepath.Join(dir, ".env")
		if _, err := os.Stat(path); err == nil {
			if err := godotenv.Load(path); err != nil {
				log.Printf("[utils] LoadEnv(%s): %v", path, err)
			}
			return
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
}
