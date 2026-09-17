package main

import (
	"os"
	"strings"
)

const (
	envDreamURL = "DREAM_URL"

	envDreamToken = "DREAM_TOKEN"

	envDreamSelfUpdate = "DREAM_SELF_UPDATE"
)

func dreamURLFromEnv() string {
	return strings.TrimSpace(os.Getenv(envDreamURL))
}

func dreamTokenFromEnv() string {
	return strings.TrimSpace(os.Getenv(envDreamToken))
}

func selfUpdateDefault() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(envDreamSelfUpdate))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func serverAddrOrEnv(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	return dreamURLFromEnv()
}
