package testutil

import (
	"context"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// MongoTestContainer represents a test MongoDB instance
type MongoTestContainer struct {
	Client *mongo.Client
	DBName string
}

// NewMongoTestContainer creates a new MongoDB test container
// In a real implementation, this would use testcontainers-go
// For now, it connects to the local test database
func NewMongoTestContainer(t *testing.T) *MongoTestContainer {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Connect to local MongoDB (assumes it's running)
	client, err := mongo.Connect(ctx, options.Client().
		ApplyURI("mongodb://root:root@localhost:27017/?authSource=admin"))
	if err != nil {
		t.Skipf("MongoDB not available for testing: %v", err)
		return nil
	}

	// Ping to verify connection
	if err := client.Ping(ctx, nil); err != nil {
		t.Skipf("MongoDB not responding: %v", err)
		return nil
	}

	// Use a unique database name for each test
	dbName := "aex_test_" + time.Now().Format("20060102_150405_000000")

	return &MongoTestContainer{
		Client: client,
		DBName: dbName,
	}
}

// Cleanup removes the test database and closes the connection
func (m *MongoTestContainer) Cleanup(t *testing.T) {
	t.Helper()

	if m == nil || m.Client == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Drop the test database
	if err := m.Client.Database(m.DBName).Drop(ctx); err != nil {
		t.Logf("Warning: failed to drop test database %s: %v", m.DBName, err)
	}

	// Close the connection
	if err := m.Client.Disconnect(ctx); err != nil {
		t.Logf("Warning: failed to disconnect from MongoDB: %v", err)
	}
}

// GetDatabase returns the test database
func (m *MongoTestContainer) GetDatabase() *mongo.Database {
	return m.Client.Database(m.DBName)
}

// AssertNoError fails the test if err is not nil
func AssertNoError(t *testing.T, err error, msgAndArgs ...interface{}) {
	t.Helper()
	if err != nil {
		if len(msgAndArgs) > 0 {
			t.Fatalf("Expected no error, got: %v - %v", err, msgAndArgs)
		} else {
			t.Fatalf("Expected no error, got: %v", err)
		}
	}
}

// AssertError fails the test if err is nil
func AssertError(t *testing.T, err error, msgAndArgs ...interface{}) {
	t.Helper()
	if err == nil {
		if len(msgAndArgs) > 0 {
			t.Fatalf("Expected error, got nil - %v", msgAndArgs)
		} else {
			t.Fatal("Expected error, got nil")
		}
	}
}

// AssertEqual fails the test if expected != actual
func AssertEqual(t *testing.T, expected, actual interface{}, msgAndArgs ...interface{}) {
	t.Helper()
	if expected != actual {
		if len(msgAndArgs) > 0 {
			t.Fatalf("Expected %v, got %v - %v", expected, actual, msgAndArgs)
		} else {
			t.Fatalf("Expected %v, got %v", expected, actual)
		}
	}
}

// AssertNotEqual fails the test if expected == actual
func AssertNotEqual(t *testing.T, expected, actual interface{}, msgAndArgs ...interface{}) {
	t.Helper()
	if expected == actual {
		if len(msgAndArgs) > 0 {
			t.Fatalf("Expected values to be different, both are %v - %v", expected, msgAndArgs)
		} else {
			t.Fatalf("Expected values to be different, both are %v", expected)
		}
	}
}

// AssertTrue fails the test if condition is false
func AssertTrue(t *testing.T, condition bool, msgAndArgs ...interface{}) {
	t.Helper()
	if !condition {
		if len(msgAndArgs) > 0 {
			t.Fatalf("Expected true, got false - %v", msgAndArgs)
		} else {
			t.Fatal("Expected true, got false")
		}
	}
}

// AssertFalse fails the test if condition is true
func AssertFalse(t *testing.T, condition bool, msgAndArgs ...interface{}) {
	t.Helper()
	if condition {
		if len(msgAndArgs) > 0 {
			t.Fatalf("Expected false, got true - %v", msgAndArgs)
		} else {
			t.Fatal("Expected false, got true")
		}
	}
}

// AssertContains fails the test if substring is not in str
func AssertContains(t *testing.T, str, substring string, msgAndArgs ...interface{}) {
	t.Helper()
	if !contains(str, substring) {
		if len(msgAndArgs) > 0 {
			t.Fatalf("Expected %q to contain %q - %v", str, substring, msgAndArgs)
		} else {
			t.Fatalf("Expected %q to contain %q", str, substring)
		}
	}
}

func contains(str, substring string) bool {
	return len(str) >= len(substring) && (str == substring || len(substring) == 0 || findSubstring(str, substring))
}

func findSubstring(str, substring string) bool {
	for i := 0; i <= len(str)-len(substring); i++ {
		if str[i:i+len(substring)] == substring {
			return true
		}
	}
	return false
}
