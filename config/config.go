package config

import (
	"fmt"
	"log"
	"os"
	"strings"
)

var (
	CLIENT_ID           = "952643138919-jh0117ivtbqkc9njoh91csm7s465c4na.apps.googleusercontent.com"
	REGION              = "us-central1"
	PROJECT             = "heroic-muse-88515"
	DATASTORE_NAMESPACE = "blog"
	HOST                = fmt.Sprintf("https://%s-%s.cloudfunctions.net", REGION, PROJECT)
	ADMINS              = []string{"joe.gregorio@gmail.com"}
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
