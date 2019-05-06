package config

import (
	"log"
	"os"
	"strings"
)

// These values are over-ridden by the values in `config.mk`, see
// README.md for an explaination of their values.
var (
	CLIENT_ID           = "952....googleusercontent.com"
	REGION              = "us-central1"
	PROJECT             = "my-project-name"
	DATASTORE_NAMESPACE = "blog"
	HOST                = "https://webmention-...-a.run.app"
	ADMINS              = []string{"someone@example.org"}
	PORT                = "8000"
)

func mustFindEnv(key string) string {
	value, ok := os.LookupEnv(key)
	if !ok {
		log.Fatalf("Failed to find environment variable %q", key)
	}
	return value
}

func init() {
	CLIENT_ID = mustFindEnv("CLIENT_ID")
	REGION = mustFindEnv("REGION")
	PROJECT = mustFindEnv("PROJECT")
	DATASTORE_NAMESPACE = mustFindEnv("DATASTORE_NAMESPACE")
	HOST = mustFindEnv("HOST")
	ADMINS = strings.Split(mustFindEnv("ADMINS"), ",")
	PORT = mustFindEnv("PORT")
}
