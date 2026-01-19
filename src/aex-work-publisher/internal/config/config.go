package config

import (
	"fmt"
	"os"
)

type Config struct {
	Port                string
	Environment         string
	StoreType           string
	MongoURI            string
	MongoDB             string
	MongoCollection     string
	FirestoreProjectID  string
	FirestoreCollection string
	ProviderRegistryURL string
}

func Load() (*Config, error) {
	cfg := &Config{
		Port:                getEnv("PORT", "8080"),
		Environment:         getEnv("ENVIRONMENT", "development"),
		StoreType:           getEnv("STORE_TYPE", "mongo"),
		MongoURI:            getEnv("MONGO_URI", "mongodb://localhost:27017"),
		MongoDB:             getEnv("MONGO_DB", "aex"),
		MongoCollection:     getEnv("MONGO_COLLECTION_WORK", "work_specs"),
		FirestoreProjectID:  getEnv("FIRESTORE_PROJECT_ID", ""),
		FirestoreCollection: getEnv("FIRESTORE_COLLECTION_WORK", "work_specs"),
		ProviderRegistryURL: getEnv("PROVIDER_REGISTRY_URL", "http://localhost:8086"),
	}

	if cfg.Environment == "production" && cfg.StoreType == "firestore" && cfg.FirestoreProjectID == "" {
		return nil, fmt.Errorf("FIRESTORE_PROJECT_ID is required in production with firestore store")
	}

	return cfg, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
