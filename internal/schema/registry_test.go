package schema

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"schemaregistry/internal/schema/types"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	h := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})
	slog.SetDefault(slog.New(h))
	os.Exit(m.Run())
}

func setupTestNATS(t *testing.T) (*server.Server, *nats.Conn, nats.KeyValue, nats.KeyValue) {
	// Create a new NATS server with custom port and JetStream enabled
	opts := &server.Options{
		Port:      19999,
		JetStream: true,
		StoreDir:  t.TempDir(), // Use a temporary directory for storage
	}
	ns, err := server.NewServer(opts)
	require.NoError(t, err)
	go ns.Start()

	// Wait for server to be ready
	if !ns.ReadyForConnections(10 * time.Second) {
		t.Fatal("NATS server failed to start")
	}

	// Connect to the server
	nc, err := nats.Connect(ns.ClientURL())
	require.NoError(t, err)

	// Create JetStream context
	js, err := nc.JetStream()
	require.NoError(t, err)

	// Wait for JetStream to be ready
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			t.Fatal("JetStream not ready in time")
		default:
			_, err := js.AccountInfo()
			if err == nil {
				// Create KV buckets
				kvSchemas, err := js.CreateKeyValue(&nats.KeyValueConfig{
					Bucket: "schemas",
				})
				require.NoError(t, err)

				kvConfig, err := js.CreateKeyValue(&nats.KeyValueConfig{
					Bucket: "config",
				})
				require.NoError(t, err)

				return ns, nc, kvSchemas, kvConfig
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func setupRegistry(t *testing.T) (*Registry, func()) {
	ns, nc, kvSchemas, kvConfig := setupTestNATS(t)
	registry := New(kvSchemas, kvConfig)

	// Wait for registry to be ready
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := registry.WaitReady(ctx)
	require.NoError(t, err)

	cleanup := func() {
		ns.Shutdown()
		nc.Close()
	}

	return registry, cleanup
}

func TestRegistry_RegisterSchema(t *testing.T) {
	registry, cleanup := setupRegistry(t)
	defer cleanup()

	tests := []struct {
		name       string
		subject    string
		schema     string
		schemaType types.SchemaType
		wantErr    bool
	}{
		{
			name:       "Valid JSON Schema",
			subject:    "test-subject",
			schema:     `{"type": "object", "properties": {"name": {"type": "string"}}}`,
			schemaType: types.JSON,
			wantErr:    false,
		},
		{
			name:       "Valid Avro Schema",
			subject:    "test-subject",
			schema:     `{"type": "record", "name": "User", "fields": [{"name": "name", "type": "string"}]}`,
			schemaType: types.Avro,
			wantErr:    false,
		},
		{
			name:       "Invalid Schema",
			subject:    "test-subject",
			schema:     `{"invalid": "schema"`,
			schemaType: types.JSON,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := registry.RegisterSchema(tt.subject, tt.schema, tt.schemaType)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Greater(t, id, 0)

			// Verify schema was stored
			schema, err := registry.GetSchema(id)
			assert.NoError(t, err)
			assert.Equal(t, tt.schema, schema.Schema)
			assert.Equal(t, tt.schemaType, schema.Type)
		})
	}
}

func TestRegistry_GetSchemaBySubjectVersion(t *testing.T) {
	registry, cleanup := setupRegistry(t)
	defer cleanup()

	// Register a test schema
	schema := `{"type": "object", "properties": {"name": {"type": "string"}}}`
	id, err := registry.RegisterSchema("test-subject", schema, types.JSON)
	require.NoError(t, err)

	tests := []struct {
		name    string
		subject string
		version string
		wantErr bool
	}{
		{
			name:    "Valid Version",
			subject: "test-subject",
			version: "1",
			wantErr: false,
		},
		{
			name:    "Latest Version",
			subject: "test-subject",
			version: "latest",
			wantErr: false,
		},
		{
			name:    "Non-existent Subject",
			subject: "non-existent",
			version: "1",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema, err := registry.GetSchemaBySubjectVersion(tt.subject, tt.version)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, id, schema.ID)
			assert.Equal(t, "test-subject", schema.Subject)
		})
	}
}

func TestRegistry_Compatibility(t *testing.T) {
	registry, cleanup := setupRegistry(t)
	defer cleanup()

	// Register initial schema
	initialSchema := `{"type": "object", "properties": {"name": {"type": "string"}}}`
	_, err := registry.RegisterSchema("test-subject", initialSchema, types.JSON)
	require.NoError(t, err)

	tests := []struct {
		name       string
		newSchema  string
		level      types.CompatibilityLevel
		wantCompat bool
		wantErr    bool
	}{
		{
			name:       "Compatible Schema - Backward",
			newSchema:  `{"type": "object", "properties": {"name": {"type": "string"}, "age": {"type": "integer"}}}`,
			level:      types.Backward,
			wantCompat: true,
			wantErr:    false,
		},
		{
			name:       "Incompatible Schema - Backward",
			newSchema:  `{"type": "object", "properties": {"name": {"type": "integer"}}}`,
			level:      types.Backward,
			wantCompat: false,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			compat, err := registry.CheckCompatibility("test-subject", tt.newSchema, types.JSON, tt.level)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.wantCompat, compat)
		})
	}
}

func TestRegistry_DeleteOperations(t *testing.T) {
	registry, cleanup := setupRegistry(t)
	defer cleanup()

	// Register test schemas
	schema1 := `{"type": "object", "properties": {"name": {"type": "string"}}}`
	schema2 := `{"type": "object", "properties": {"age": {"type": "integer"}}}`

	_, err := registry.RegisterSchema("test-subject", schema1, types.JSON)
	require.NoError(t, err)
	id2, err := registry.RegisterSchema("test-subject", schema2, types.JSON)
	require.NoError(t, err)

	t.Run("Delete Schema Version", func(t *testing.T) {
		err := registry.DeleteSchemaVersion("test-subject", "1")
		assert.NoError(t, err)

		// Verify schema is deleted
		_, err = registry.GetSchemaBySubjectVersion("test-subject", "1")
		assert.Error(t, err)
	})

	t.Run("Delete Subject", func(t *testing.T) {
		deletedIDs, err := registry.DeleteSubject("test-subject")
		assert.NoError(t, err)
		assert.Equal(t, []int{id2}, deletedIDs)

		// Verify subject is deleted
		versions, err := registry.GetVersions("test-subject")
		assert.Error(t, err)
		assert.Nil(t, versions)
	})
}
