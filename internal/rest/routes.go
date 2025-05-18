package rest

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"schemaregistry/internal/schema"
	"schemaregistry/internal/schema/types"

	"github.com/gin-gonic/gin"
	"github.com/nats-io/nats.go"
)

var registry *schema.Registry
var kvSchemas, kvConfig nats.KeyValue

// MemoryKeyValue is a simple in-memory implementation of the nats.KeyValue interface
// for fallback when NATS is not available
type MemoryKeyValue struct {
	name  string
	data  map[string][]byte
	mutex sync.RWMutex
}

// NewMemoryKeyValue creates a new in-memory KeyValue store
func NewMemoryKeyValue(name string) *MemoryKeyValue {
	return &MemoryKeyValue{
		name: name,
		data: make(map[string][]byte),
	}
}

// Get retrieves a value for a key
func (m *MemoryKeyValue) Get(key string) (nats.KeyValueEntry, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	if val, ok := m.data[key]; ok {
		return &MemoryKeyValueEntry{
			key:   key,
			value: val,
		}, nil
	}
	return nil, nats.ErrKeyNotFound
}

// GetRevision returns a specific revision value for the key
func (m *MemoryKeyValue) GetRevision(key string, revision uint64) (nats.KeyValueEntry, error) {
	// In-memory implementation doesn't track revisions, so just return the current value
	return m.Get(key)
}

// Put stores a value for a key
func (m *MemoryKeyValue) Put(key string, value []byte) (uint64, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.data[key] = value
	return 1, nil // Return a dummy revision
}

// PutString stores a string value for a key
func (m *MemoryKeyValue) PutString(key string, value string) (uint64, error) {
	return m.Put(key, []byte(value))
}

// Keys returns all keys in the store
func (m *MemoryKeyValue) Keys(opts ...nats.WatchOpt) ([]string, error) {
	slog.Debug("MemoryKeyValue.Keys called", "bucket", m.name)

	m.mutex.RLock()
	defer m.mutex.RUnlock()

	// Initialize with empty slice to avoid returning nil
	keys := make([]string, 0)

	if len(m.data) == 0 {
		slog.Debug("No keys in memory store", "bucket", m.name)
		return keys, nil
	}

	// Add keys to the slice
	for k := range m.data {
		slog.Debug("Adding key", "key", k)
		keys = append(keys, k)
	}

	slog.Debug("Returning keys", "count", len(keys), "bucket", m.name)
	return keys, nil
}

// ListKeys returns all keys in the store via a channel
func (m *MemoryKeyValue) ListKeys(opts ...nats.WatchOpt) (nats.KeyLister, error) {
	// Not implemented for in-memory store
	return nil, fmt.Errorf("list keys not implemented for in-memory store")
}

// Create creates a new key with the given value
func (m *MemoryKeyValue) Create(key string, value []byte) (uint64, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if _, ok := m.data[key]; ok {
		return 0, nats.ErrKeyExists
	}

	m.data[key] = value
	return 1, nil // Return a dummy revision
}

// Update updates a key with the given value and revision
func (m *MemoryKeyValue) Update(key string, value []byte, last uint64) (uint64, error) {
	// In-memory implementation doesn't track revisions
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if _, ok := m.data[key]; !ok {
		return 0, nats.ErrKeyNotFound
	}

	m.data[key] = value
	return last + 1, nil
}

// Delete deletes a key
func (m *MemoryKeyValue) Delete(key string, opts ...nats.DeleteOpt) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if _, ok := m.data[key]; !ok {
		return nats.ErrKeyNotFound
	}

	delete(m.data, key)
	return nil
}

// Purge purges a key
func (m *MemoryKeyValue) Purge(key string, opts ...nats.DeleteOpt) error {
	return m.Delete(key, opts...) // Same as delete for in-memory
}

// Watch watches for changes to keys
func (m *MemoryKeyValue) Watch(keys string, opts ...nats.WatchOpt) (nats.KeyWatcher, error) {
	// Not implemented for in-memory store
	return nil, fmt.Errorf("watch not implemented for in-memory store")
}

// WatchAll watches for changes to all keys
func (m *MemoryKeyValue) WatchAll(opts ...nats.WatchOpt) (nats.KeyWatcher, error) {
	// Not implemented for in-memory store
	return nil, fmt.Errorf("watch all not implemented for in-memory store")
}

// WatchFiltered watches for changes to keys matching the filter
func (m *MemoryKeyValue) WatchFiltered(keys []string, opts ...nats.WatchOpt) (nats.KeyWatcher, error) {
	// Not implemented for in-memory store
	return nil, fmt.Errorf("watch filtered not implemented for in-memory store")
}

// History returns the history for a key
func (m *MemoryKeyValue) History(key string, opts ...nats.WatchOpt) ([]nats.KeyValueEntry, error) {
	// Not implemented for in-memory store
	return nil, fmt.Errorf("history not implemented for in-memory store")
}

// Bucket returns the bucket name
func (m *MemoryKeyValue) Bucket() string {
	return m.name
}

// PurgeDeletes purges deleted keys
func (m *MemoryKeyValue) PurgeDeletes(opts ...nats.PurgeOpt) error {
	// Not implemented for in-memory store
	return nil
}

// Status returns the status of the bucket
func (m *MemoryKeyValue) Status() (nats.KeyValueStatus, error) {
	// Create a memory status implementation
	status := &MemoryKeyValueStatus{
		bucket:       m.name,
		values:       uint64(len(m.data)),
		backingStore: "Memory",
	}
	return status, nil
}

// MemoryKeyValueEntry implements the KeyValueEntry interface for the in-memory store
type MemoryKeyValueEntry struct {
	key   string
	value []byte
}

// Bucket returns the bucket name
func (e *MemoryKeyValueEntry) Bucket() string {
	return ""
}

// Key returns the key
func (e *MemoryKeyValueEntry) Key() string {
	return e.key
}

// Value returns the value
func (e *MemoryKeyValueEntry) Value() []byte {
	return e.value
}

// Revision returns the revision
func (e *MemoryKeyValueEntry) Revision() uint64 {
	return 1
}

// Created returns the creation time
func (e *MemoryKeyValueEntry) Created() time.Time {
	return time.Now()
}

// Delta returns the delta
func (e *MemoryKeyValueEntry) Delta() uint64 {
	return 0
}

// Operation returns the operation
func (e *MemoryKeyValueEntry) Operation() nats.KeyValueOp {
	return nats.KeyValuePut
}

// MemoryKeyValueStatus implements the KeyValueStatus interface
type MemoryKeyValueStatus struct {
	bucket       string
	values       uint64
	backingStore string
}

// Bucket returns the bucket name
func (s *MemoryKeyValueStatus) Bucket() string {
	return s.bucket
}

// Values returns the number of values in the bucket
func (s *MemoryKeyValueStatus) Values() uint64 {
	return s.values
}

// History returns the configured history kept per key
func (s *MemoryKeyValueStatus) History() int64 {
	return 1 // No history in memory implementation
}

// TTL returns how long the bucket keeps values for
func (s *MemoryKeyValueStatus) TTL() time.Duration {
	return 0 // No expiration in memory implementation
}

// BackingStore returns the technology used for storage
func (s *MemoryKeyValueStatus) BackingStore() string {
	return s.backingStore
}

// Bytes returns the size in bytes of the bucket
func (s *MemoryKeyValueStatus) Bytes() uint64 {
	return 0 // Not tracking bytes in memory implementation
}

// IsCompressed returns if the data is compressed
func (s *MemoryKeyValueStatus) IsCompressed() bool {
	return false // No compression in memory implementation
}

// Init initializes the REST handlers with the schema registry
// If NATS is not available, it will use in-memory implementations
func Init(schemas, config nats.KeyValue) {
	slog.Info("Initializing schema registry handlers")

	// If both KeyValue stores are nil, use in-memory fallbacks
	if schemas == nil {
		slog.Warn("Schema storage not available, using in-memory fallback")
		schemas = NewMemoryKeyValue("SCHEMAS")
	} else {
		slog.Info("Using external schema storage", "bucket", schemas.Bucket())
	}

	if config == nil {
		slog.Warn("Config storage not available, using in-memory fallback")
		config = NewMemoryKeyValue("CONFIG")
	} else {
		slog.Info("Using external config storage", "bucket", config.Bucket())
	}

	kvSchemas = schemas
	kvConfig = config

	// Create registry with the storage
	slog.Debug("Creating schema registry")
	registry = schema.New(kvSchemas, kvConfig)

	slog.Info("Schema registry handlers initialized successfully")
}

// SchemaRecord represents a stored schema record
type SchemaRecord struct {
	Schema     string `json:"schema"`
	Subject    string `json:"subject"`
	Version    int    `json:"version"`
	ID         int    `json:"id"`
	SchemaType string `json:"schemaType,omitempty"`
}

// SchemaRequest is payload for registering schemas.
type SchemaRequest struct {
	Schema     string `json:"schema"`
	SchemaType string `json:"schemaType,omitempty"`
}

// SchemaResponse returns the schema ID.
type SchemaResponse struct {
	ID int `json:"id"`
}

// CompatibilityResponse indicates compatibility result.
type CompatibilityResponse struct {
	IsCompatible bool `json:"is_compatible"`
}

// ConfigRequest updates compatibility.
type ConfigRequest struct {
	Compatibility string `json:"compatibility"`
}

// ConfigResponse returns compatibility.
type ConfigResponse struct {
	CompatibilityLevel string `json:"compatibilityLevel"`
}

// ErrorResponse represents an error message
type ErrorResponse struct {
	ErrorCode int    `json:"error_code"`
	Message   string `json:"message"`
}

// SetupRouter creates and configures a Gin router with all schema registry routes
func SetupRouter() *gin.Engine {
	// Set Gin to release mode in production
	gin.SetMode(gin.ReleaseMode)

	r := gin.New()
	r.Use(gin.Recovery())

	// Set custom content type for all responses
	r.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Content-Type", "application/vnd.schemaregistry.v1+json")
		c.Next()
	})

	// Subjects routes
	r.GET("/subjects", handleSubjects)

	// Subject versions routes
	subjectGroup := r.Group("/subjects/:subject")
	{
		subjectGroup.GET("/versions", listVersions)
		subjectGroup.POST("/versions", registerSchema)
		subjectGroup.GET("/versions/:version", getSchema)
		subjectGroup.DELETE("/versions/:version", deleteSchemaVersion)
		subjectGroup.DELETE("", deleteSubject)
		subjectGroup.POST("", checkSchema)
	}

	// Schema ID routes
	r.GET("/schemas/ids/:id", getSchemaById)

	// Compatibility routes
	r.POST("/compatibility/subjects/:subject/versions/:version", checkCompatibility)
	r.POST("/compatibility/subjects/:subject/versions", checkCompatibilityForSubject)

	// Config routes
	r.GET("/config", getGlobalConfig)
	r.PUT("/config", updateGlobalConfig)
	r.GET("/config/:subject", getSubjectConfig)
	r.PUT("/config/:subject", updateSubjectConfig)

	return r
}

// Routes returns an http.Handler for backward compatibility
func Routes() http.Handler {
	return SetupRouter()
}

func handleSubjects(c *gin.Context) {
	// Check if storage is available
	if kvSchemas == nil {
		slog.Error("Storage not available", "endpoint", "handleSubjects")
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{
			ErrorCode: 50300,
			Message:   "storage backend unavailable",
		})
		return
	}

	// Get all subjects with at least one version
	slog.Debug("Getting keys from KeyValue store", "bucket", kvSchemas.Bucket())
	keys, err := kvSchemas.Keys()
	if err != nil {
		slog.Error("Failed to get keys", "error", err, "bucket", kvSchemas.Bucket())
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			ErrorCode: 50000,
			Message:   fmt.Sprintf("failed to get keys: %v", err),
		})
		return
	}

	// Filter out internal keys and extract unique subjects
	subjects := make(map[string]bool)
	prefix := "subjects/"
	for _, key := range keys {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		parts := strings.Split(strings.TrimPrefix(key, prefix), "/")
		if len(parts) > 0 {
			subjects[parts[0]] = true
		}
	}

	// Convert map to slice
	subjectList := make([]string, 0, len(subjects))
	for subject := range subjects {
		subjectList = append(subjectList, subject)
	}

	slog.Debug("Got subjects", "count", len(subjectList), "subjects", subjectList)
	c.JSON(http.StatusOK, subjectList)
}

func registerSchema(c *gin.Context) {
	subject := c.Param("subject")

	// Check if storage is available
	if kvSchemas == nil || registry == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{
			ErrorCode: 50300,
			Message:   "storage backend unavailable",
		})
		return
	}

	var req SchemaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			ErrorCode: 42201,
			Message:   "invalid JSON",
		})
		return
	}

	schemaType := types.Avro
	if req.SchemaType != "" {
		schemaType = types.SchemaType(req.SchemaType)
	}

	slog.Debug("Registering schema", "subject", subject, "schema", req.Schema, "schemaType", schemaType)
	id, err := registry.RegisterSchema(subject, req.Schema, schemaType)
	if err != nil {
		if err.Error() == "incompatible schema" {
			c.JSON(http.StatusConflict, ErrorResponse{
				ErrorCode: 40901,
				Message:   "incompatible schema",
			})
		} else {
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				ErrorCode: 50000,
				Message:   err.Error(),
			})
		}
		return
	}

	c.JSON(http.StatusOK, SchemaResponse{ID: id})
}

func getSchema(c *gin.Context) {
	subject := c.Param("subject")
	version := c.Param("version")

	// Check if storage is available
	if kvSchemas == nil || registry == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{
			ErrorCode: 50300,
			Message:   "storage backend unavailable",
		})
		return
	}

	schema, err := registry.GetSchemaBySubjectVersion(subject, version)
	if err != nil {
		code := http.StatusInternalServerError
		if err.Error() == "no versions found" {
			code = http.StatusNotFound
		}

		c.JSON(code, ErrorResponse{
			ErrorCode: 40401,
			Message:   err.Error(),
		})
		return
	}

	// Convert schema to the response format
	response := SchemaRecord{
		Schema:  schema.Schema,
		Subject: schema.Subject,
		Version: schema.Version,
		ID:      schema.ID,
	}

	// Only include schemaType if not default (Avro)
	if schema.Type != types.Avro {
		response.SchemaType = string(schema.Type)
	}

	c.JSON(http.StatusOK, response)
}

func listVersions(c *gin.Context) {
	slog.Debug("Listing versions")
	subject := c.Param("subject")

	// Check if storage is available
	if kvSchemas == nil || registry == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{
			ErrorCode: 50300,
			Message:   "storage backend unavailable",
		})
		return
	}

	versions, err := registry.GetVersions(subject)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			ErrorCode: 50000,
			Message:   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, versions)
}

func checkCompatibility(c *gin.Context) {
	subject := c.Param("subject")

	// Check if storage is available
	if kvSchemas == nil || registry == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{
			ErrorCode: 50300,
			Message:   "storage backend unavailable",
		})
		return
	}

	var req SchemaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			ErrorCode: 42201,
			Message:   "invalid JSON",
		})
		return
	}

	schemaType := types.Avro
	if req.SchemaType != "" {
		schemaType = types.SchemaType(req.SchemaType)
	}

	level, err := registry.GetCompatibilityLevel(subject)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			ErrorCode: 50000,
			Message:   err.Error(),
		})
		return
	}

	compatible, err := registry.CheckCompatibility(subject, req.Schema, schemaType, level)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			ErrorCode: 50000,
			Message:   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, CompatibilityResponse{IsCompatible: compatible})
}

func checkCompatibilityForSubject(c *gin.Context) {
	subject := c.Param("subject")

	// Check if storage is available
	if kvSchemas == nil || registry == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{
			ErrorCode: 50300,
			Message:   "storage backend unavailable",
		})
		return
	}

	var req SchemaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			ErrorCode: 42201,
			Message:   "invalid JSON",
		})
		return
	}

	schemaType := types.Avro
	if req.SchemaType != "" {
		schemaType = types.SchemaType(req.SchemaType)
	}

	level, err := registry.GetCompatibilityLevel(subject)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			ErrorCode: 50000,
			Message:   err.Error(),
		})
		return
	}

	compatible, err := registry.CheckCompatibility(subject, req.Schema, schemaType, level)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			ErrorCode: 50000,
			Message:   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, CompatibilityResponse{IsCompatible: compatible})
}

func getGlobalConfig(c *gin.Context) {
	// Check if storage is available
	if kvConfig == nil || registry == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{
			ErrorCode: 50300,
			Message:   "storage backend unavailable",
		})
		return
	}

	level, err := registry.GetCompatibilityLevel("global")
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			ErrorCode: 50000,
			Message:   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, ConfigResponse{CompatibilityLevel: string(level)})
}

func updateGlobalConfig(c *gin.Context) {
	// Check if storage is available
	if kvConfig == nil || registry == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{
			ErrorCode: 50300,
			Message:   "storage backend unavailable",
		})
		return
	}

	var req ConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			ErrorCode: 42201,
			Message:   "invalid JSON",
		})
		return
	}

	if err := registry.SetCompatibilityLevel("global", types.CompatibilityLevel(req.Compatibility)); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			ErrorCode: 50000,
			Message:   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, ConfigResponse{CompatibilityLevel: req.Compatibility})
}

func getSubjectConfig(c *gin.Context) {
	subject := c.Param("subject")

	// Check if storage is available
	if kvConfig == nil || registry == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{
			ErrorCode: 50300,
			Message:   "storage backend unavailable",
		})
		return
	}

	level, err := registry.GetCompatibilityLevel(subject)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			ErrorCode: 50000,
			Message:   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, ConfigResponse{CompatibilityLevel: string(level)})
}

func updateSubjectConfig(c *gin.Context) {
	subject := c.Param("subject")

	// Check if storage is available
	if kvConfig == nil || registry == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{
			ErrorCode: 50300,
			Message:   "storage backend unavailable",
		})
		return
	}

	var req ConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			ErrorCode: 42201,
			Message:   "invalid JSON",
		})
		return
	}

	if err := registry.SetCompatibilityLevel(subject, types.CompatibilityLevel(req.Compatibility)); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			ErrorCode: 50000,
			Message:   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, ConfigResponse{CompatibilityLevel: req.Compatibility})
}

func getSchemaById(c *gin.Context) {
	id := c.Param("id")

	// Check if storage is available
	if kvSchemas == nil || registry == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{
			ErrorCode: 50300,
			Message:   "storage backend unavailable",
		})
		return
	}

	schema, err := registry.GetSchemaById(id)
	if err != nil {
		code := http.StatusInternalServerError
		if err.Error() == "schema not found" {
			code = http.StatusNotFound
		}

		c.JSON(code, ErrorResponse{
			ErrorCode: 40403,
			Message:   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, map[string]string{"schema": schema.Schema})
}

func deleteSchemaVersion(c *gin.Context) {
	subject := c.Param("subject")
	version := c.Param("version")

	// Check if storage is available
	if kvSchemas == nil || registry == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{
			ErrorCode: 50300,
			Message:   "storage backend unavailable",
		})
		return
	}

	err := registry.DeleteSchemaVersion(subject, version)
	if err != nil {
		code := http.StatusInternalServerError
		if err.Error() == "version not found" {
			code = http.StatusNotFound
		}

		c.JSON(code, ErrorResponse{
			ErrorCode: 40402,
			Message:   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, version)
}

func deleteSubject(c *gin.Context) {
	subject := c.Param("subject")

	// Check if storage is available
	if kvSchemas == nil || registry == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{
			ErrorCode: 50300,
			Message:   "storage backend unavailable",
		})
		return
	}

	versions, err := registry.DeleteSubject(subject)
	if err != nil {
		code := http.StatusInternalServerError
		if err.Error() == "subject not found" {
			code = http.StatusNotFound
		}

		c.JSON(code, ErrorResponse{
			ErrorCode: 40401,
			Message:   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, versions)
}

func checkSchema(c *gin.Context) {
	subject := c.Param("subject")
	slog.Debug("Checking schema", "subject", subject)

	// Check if storage is available
	if kvSchemas == nil || registry == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{
			ErrorCode: 50300,
			Message:   "storage backend unavailable",
		})
		return
	}

	var req SchemaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			ErrorCode: 42201,
			Message:   "invalid JSON",
		})
		return
	}

	schemaType := types.Avro
	if req.SchemaType != "" {
		schemaType = types.SchemaType(req.SchemaType)
	}

	schema, err := registry.LookupSchema(subject, req.Schema, schemaType)
	if err != nil {
		if err.Error() == "schema not found" {
			c.JSON(http.StatusNotFound, ErrorResponse{
				ErrorCode: 40403,
				Message:   err.Error(),
			})
		} else {
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				ErrorCode: 50000,
				Message:   err.Error(),
			})
		}
		return
	}

	// Convert schema to the response format
	response := SchemaRecord{
		Schema:  schema.Schema,
		Subject: schema.Subject,
		Version: schema.Version,
		ID:      schema.ID,
	}

	// Only include schemaType if not default (Avro)
	if schema.Type != types.Avro {
		response.SchemaType = string(schema.Type)
	}

	c.JSON(http.StatusOK, response)
}
