package schema

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"sync"

	"schemaregistry/internal/schema/formats/avro"
	jsonformat "schemaregistry/internal/schema/formats/json"
	"schemaregistry/internal/schema/formats/protobuf"
	"schemaregistry/internal/schema/types"

	"github.com/nats-io/nats.go"
)

const (
	MagicByte = 0x0

	// Key prefixes for NATS KeyValue store
	keyPrefixSubjects      = "subjects/"        // subjects/{subject}/versions/{version}
	keyPrefixSchemas       = "schemas/"         // schemas/{id}
	keyPrefixGlobalConfig  = "config/global"    // global config
	keyPrefixSubjectConfig = "config/subjects/" // config/subjects/{subject}

	// Default compatibility level
	defaultCompatibilityLevel = types.Backward
)

// WireFormat represents the serialized format of a message
type WireFormat struct {
	MagicByte byte
	SchemaID  int32
	Data      []byte
}

// cacheEntry represents a cached schema with its metadata
type cacheEntry struct {
	schema *types.Schema
	mu     sync.RWMutex
}

// Registry manages schema registration and compatibility checking
type Registry struct {
	formats   map[types.SchemaType]types.SchemaFormat
	kvSchemas nats.KeyValue
	kvConfig  nats.KeyValue
	mu        sync.RWMutex

	// Cache layer
	schemaCache  map[int]*cacheEntry    // schema ID -> schema
	subjectCache map[string][]int       // subject -> version list
	versionCache map[string]map[int]int // subject -> version -> schema ID
	configCache  map[string][]byte      // subject -> compatibility level
	watchSub     *nats.Subscription     // NATS subscription for updates
	stopWatch    chan struct{}          // Channel to stop watching
	ready        chan struct{}          // Channel to signal when ready
}

// New creates a new schema registry
func New(kvSchemas, kvConfig nats.KeyValue) *Registry {
	r := &Registry{
		formats: map[types.SchemaType]types.SchemaFormat{
			types.JSON:     jsonformat.New(),
			types.Avro:     avro.New(),
			types.Protobuf: protobuf.New(),
		},
		kvSchemas:    kvSchemas,
		kvConfig:     kvConfig,
		schemaCache:  make(map[int]*cacheEntry),
		subjectCache: make(map[string][]int),
		versionCache: make(map[string]map[int]int),
		configCache:  make(map[string][]byte),
		stopWatch:    make(chan struct{}),
		ready:        make(chan struct{}),
	}

	// Start watching for updates
	go r.watchUpdates()

	return r
}

// WaitReady waits for the registry to be ready
func (r *Registry) WaitReady(ctx context.Context) error {
	select {
	case <-r.ready:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// watchUpdates watches for changes in the NATS KeyValue store
func (r *Registry) watchUpdates() {
	// Watch for schema updates
	schemaWatcher, err := r.kvSchemas.WatchAll()
	if err != nil {
		slog.Error("Failed to watch schema updates", "error", err)
		return
	}
	defer schemaWatcher.Stop()

	// Watch for config updates
	configWatcher, err := r.kvConfig.WatchAll()
	if err != nil {
		slog.Error("Failed to watch config updates", "error", err)
		return
	}
	defer configWatcher.Stop()

	// Signal that we're ready to process updates
	close(r.ready)

	for {
		select {
		case <-r.stopWatch:
			return
		case update := <-schemaWatcher.Updates():
			if update == nil {
				continue
			}
			r.handleSchemaUpdate(update)
		case update := <-configWatcher.Updates():
			if update == nil {
				continue
			}
			r.handleConfigUpdate(update)
		}
	}
}

// handleSchemaUpdate processes schema updates from NATS
func (r *Registry) handleSchemaUpdate(update nats.KeyValueEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := update.Key()
	value := update.Value()

	// Handle schema deletion
	if update.Operation() == nats.KeyValueDelete {
		if strings.HasPrefix(key, keyPrefixSchemas) {
			// Remove from schema cache
			idStr := strings.TrimPrefix(key, keyPrefixSchemas)
			id, err := strconv.Atoi(idStr)
			if err == nil {
				delete(r.schemaCache, id)
			}
		} else if strings.HasPrefix(key, keyPrefixSubjects) {
			// Remove from subject and version caches
			parts := strings.Split(key, "/")
			if len(parts) >= 4 {
				subject := parts[1]
				versionStr := parts[3]
				version, err := strconv.Atoi(versionStr)
				if err == nil {
					if versions, ok := r.versionCache[subject]; ok {
						delete(versions, version)
					}
					// Update subject cache
					if versions, ok := r.subjectCache[subject]; ok {
						for i, v := range versions {
							if v == version {
								r.subjectCache[subject] = append(versions[:i], versions[i+1:]...)
								break
							}
						}
					}
				}
			}
		}
		return
	}

	// Handle schema updates
	if strings.HasPrefix(key, keyPrefixSchemas) {
		var schema types.Schema
		if err := json.Unmarshal(value, &schema); err != nil {
			slog.Error("Failed to unmarshal schema update", "error", err)
			return
		}
		r.schemaCache[schema.ID] = &cacheEntry{schema: &schema}
	} else if strings.HasPrefix(key, keyPrefixSubjects) {
		var schema types.Schema
		if err := json.Unmarshal(value, &schema); err != nil {
			slog.Error("Failed to unmarshal subject update", "error", err)
			return
		}
		// Update version cache
		if _, ok := r.versionCache[schema.Subject]; !ok {
			r.versionCache[schema.Subject] = make(map[int]int)
		}
		r.versionCache[schema.Subject][schema.Version] = schema.ID
		// Update subject cache
		versions := r.subjectCache[schema.Subject]
		found := false
		for _, v := range versions {
			if v == schema.Version {
				found = true
				break
			}
		}
		if !found {
			r.subjectCache[schema.Subject] = append(versions, schema.Version)
			sort.Ints(r.subjectCache[schema.Subject])
		}
	}
}

// handleConfigUpdate processes config updates from NATS
func (r *Registry) handleConfigUpdate(update nats.KeyValueEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := update.Key()
	value := update.Value()

	// Handle config deletion
	if update.Operation() == nats.KeyValueDelete {
		if key == keyPrefixGlobalConfig {
			delete(r.configCache, "global")
		} else if strings.HasPrefix(key, keyPrefixSubjectConfig) {
			subject := strings.TrimPrefix(key, keyPrefixSubjectConfig)
			delete(r.configCache, subject)
		}
		return
	}

	// Handle config updates
	if key == keyPrefixGlobalConfig {
		r.configCache["global"] = value
	} else if strings.HasPrefix(key, keyPrefixSubjectConfig) {
		subject := strings.TrimPrefix(key, keyPrefixSubjectConfig)
		r.configCache[subject] = value
	}
}

// RegisterSchema registers a new schema and returns its ID
func (r *Registry) RegisterSchema(subject string, schemaStr string, schemaType types.SchemaType) (int, error) {
	// Validate schema format
	format, ok := r.formats[schemaType]
	if !ok {
		return 0, fmt.Errorf("unsupported schema type: %s", schemaType)
	}

	// Validate the schema
	if err := format.Validate(schemaStr); err != nil {
		return 0, fmt.Errorf("validate schema: %w", err)
	}

	// Check if schema already exists for this subject
	latestVersion, err := r.getLatestVersion(subject)
	if err != nil && err.Error() != "no versions found" {
		return 0, fmt.Errorf("get latest version: %w", err)
	}

	if latestVersion > 0 {
		slog.Debug("Checking compatibility for schema", "subject", subject, "latestVersion", latestVersion)

		// If schema exists, check compatibility
		level, err := r.GetCompatibilityLevel(subject)
		if err != nil {
			return 0, fmt.Errorf("get compatibility level: %w", err)
		}
		slog.Debug("Compatibility level", "subject", subject, "level", level)

		// Get the latest schema for compatibility check
		latestSchema, err := r.getSchemaByVersion(subject, latestVersion)
		if err != nil {
			return 0, fmt.Errorf("get latest schema: %w", err)
		}

		// Check if schema content is identical
		if latestSchema.Schema == schemaStr && latestSchema.Type == schemaType {
			return latestSchema.ID, nil
		}

		// Check compatibility
		compatible, err := format.CheckCompatibility(latestSchema.Schema, schemaStr, level)
		if err != nil || !compatible {
			return 0, fmt.Errorf("incompatible schema: %w", err)
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if schema content already exists globally
	keys, err := r.kvSchemas.Keys()
	if err != nil && err != nats.ErrNoKeysFound {
		return 0, fmt.Errorf("get schema keys: %w", err)
	}

	var existingID int
	for _, key := range keys {
		if !strings.HasPrefix(key, keyPrefixSchemas) {
			continue
		}

		entry, err := r.kvSchemas.Get(key)
		if err != nil {
			continue
		}

		var schema types.Schema
		if err := json.Unmarshal(entry.Value(), &schema); err != nil {
			continue
		}

		if schema.Schema == schemaStr && schema.Type == schemaType {
			existingID = schema.ID
			break
		}
	}

	// If schema exists, reuse its ID
	if existingID > 0 {
		// Create new version for this subject
		newVersion := latestVersion + 1
		schema := &types.Schema{
			Schema:  schemaStr,
			Subject: subject,
			Version: newVersion,
			ID:      existingID,
			Type:    schemaType,
		}

		// Store schema by subject and version
		schemaBytes, err := json.Marshal(schema)
		if err != nil {
			return 0, fmt.Errorf("marshal schema: %w", err)
		}

		if _, err := r.kvSchemas.Put(
			fmt.Sprintf("%s%s/versions/%d", keyPrefixSubjects, subject, newVersion),
			schemaBytes,
		); err != nil {
			return 0, fmt.Errorf("store schema by subject/version: %w", err)
		}

		return existingID, nil
	}

	// Get the next schema ID for new schema
	nextID, err := r.getNextSchemaID()
	if err != nil {
		return 0, fmt.Errorf("get next schema ID: %w", err)
	}

	// Create new schema
	newVersion := latestVersion + 1
	schema := &types.Schema{
		Schema:  schemaStr,
		Subject: subject,
		Version: newVersion,
		ID:      nextID,
		Type:    schemaType,
	}

	// Store schema by ID
	schemaBytes, err := json.Marshal(schema)
	if err != nil {
		return 0, fmt.Errorf("marshal schema: %w", err)
	}

	if _, err := r.kvSchemas.Put(keyPrefixSchemas+strconv.Itoa(nextID), schemaBytes); err != nil {
		return 0, fmt.Errorf("store schema by ID: %w", err)
	}

	// Store schema by subject and version
	if _, err := r.kvSchemas.Put(
		fmt.Sprintf("%s%s/versions/%d", keyPrefixSubjects, subject, newVersion),
		schemaBytes,
	); err != nil {
		return 0, fmt.Errorf("store schema by subject/version: %w", err)
	}

	return nextID, nil
}

// getNextSchemaID gets the next available schema ID
func (r *Registry) getNextSchemaID() (int, error) {
	// Get all schemas
	keys, err := r.kvSchemas.Keys()
	if err != nil && err != nats.ErrNoKeysFound {
		return 0, err
	}

	// Find the highest ID
	highestID := 0
	for _, key := range keys {
		if !strings.HasPrefix(key, keyPrefixSchemas) {
			continue
		}

		idStr := strings.TrimPrefix(key, keyPrefixSchemas)
		id, err := strconv.Atoi(idStr)
		if err != nil {
			continue
		}

		if id > highestID {
			highestID = id
		}
	}

	// Return next ID
	return highestID + 1, nil
}

// getLatestVersion gets the latest version for a subject
func (r *Registry) getLatestVersion(subject string) (int, error) {
	prefix := fmt.Sprintf("%s%s/versions/", keyPrefixSubjects, subject)
	keys, err := r.kvSchemas.Keys()
	if err != nil && err != nats.ErrNoKeysFound {
		return 0, err
	}

	// Find highest version
	highestVersion := 0
	for _, key := range keys {
		if !strings.HasPrefix(key, prefix) {
			continue
		}

		versionStr := strings.TrimPrefix(key, prefix)
		version, err := strconv.Atoi(versionStr)
		if err != nil {
			continue
		}

		if version > highestVersion {
			highestVersion = version
		}
	}

	if highestVersion == 0 {
		return 0, nil // Return 0 for first version
	}

	return highestVersion, nil
}

// getSchemaByVersion gets a schema by subject and version
func (r *Registry) getSchemaByVersion(subject string, version int) (*types.Schema, error) {
	key := fmt.Sprintf("%s%s/versions/%d", keyPrefixSubjects, subject, version)
	entry, err := r.kvSchemas.Get(key)
	if err != nil {
		return nil, err
	}

	var schema types.Schema
	if err := json.Unmarshal(entry.Value(), &schema); err != nil {
		return nil, fmt.Errorf("unmarshal schema: %w", err)
	}

	return &schema, nil
}

// GetSchema retrieves a schema by ID
func (r *Registry) GetSchema(id int) (*types.Schema, error) {
	// Try cache first
	if entry, ok := r.schemaCache[id]; ok {
		entry.mu.RLock()
		schema := entry.schema
		entry.mu.RUnlock()
		return schema, nil
	}

	// Cache miss, get from store
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := keyPrefixSchemas + strconv.Itoa(id)
	entry, err := r.kvSchemas.Get(key)
	if err != nil {
		return nil, fmt.Errorf("schema not found: %d", id)
	}

	var schema types.Schema
	if err := json.Unmarshal(entry.Value(), &schema); err != nil {
		return nil, fmt.Errorf("unmarshal schema: %w", err)
	}

	// Update cache
	r.schemaCache[id] = &cacheEntry{schema: &schema}

	return &schema, nil
}

// GetSchemaBySubjectVersion retrieves a schema by subject and version
func (r *Registry) GetSchemaBySubjectVersion(subject string, version string) (*types.Schema, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var versionNum int
	var err error

	// Handle "latest" version
	if version == "latest" {
		versionNum, err = r.getLatestVersion(subject)
		if err != nil {
			return nil, err
		}
	} else {
		versionNum, err = strconv.Atoi(version)
		if err != nil {
			return nil, fmt.Errorf("invalid version: %s", version)
		}
	}

	return r.getSchemaByVersion(subject, versionNum)
}

// GetVersions returns all versions for a subject
func (r *Registry) GetVersions(subject string) ([]int, error) {
	// Try cache first
	if versions, ok := r.subjectCache[subject]; ok {
		return versions, nil
	}

	// Cache miss, get from store
	r.mu.RLock()
	defer r.mu.RUnlock()

	prefix := fmt.Sprintf("%s%s/versions/", keyPrefixSubjects, subject)
	keys, err := r.kvSchemas.Keys()
	if err != nil {
		return nil, err
	}

	var versions []int
	for _, key := range keys {
		if !strings.HasPrefix(key, prefix) {
			continue
		}

		versionStr := strings.TrimPrefix(key, prefix)
		version, err := strconv.Atoi(versionStr)
		if err != nil {
			continue
		}

		versions = append(versions, version)
	}

	if len(versions) == 0 {
		return nil, fmt.Errorf("no versions found")
	}

	// Update cache
	sort.Ints(versions)
	r.subjectCache[subject] = versions

	return versions, nil
}

// GetCompatibilityLevel gets the compatibility level for a subject
func (r *Registry) GetCompatibilityLevel(subject string) (types.CompatibilityLevel, error) {
	// Try cache first
	if level, ok := r.configCache[subject]; ok {
		return types.CompatibilityLevel(level), nil
	}
	if level, ok := r.configCache["global"]; ok {
		return types.CompatibilityLevel(level), nil
	}

	// Cache miss, get from store
	r.mu.RLock()
	defer r.mu.RUnlock()

	slog.Debug("Getting compatibility level", "subject", subject)
	// First try subject-specific config
	if subject != "global" {
		key := keyPrefixSubjectConfig + subject
		entry, err := r.kvConfig.Get(key)
		if err == nil {
			level := entry.Value()
			r.configCache[subject] = level
			return types.CompatibilityLevel(level), nil
		}
	}

	// Fallback to global config
	entry, err := r.kvConfig.Get(keyPrefixGlobalConfig)
	if err != nil {
		// Use default if global config not found
		return defaultCompatibilityLevel, nil
	}

	level := entry.Value()
	r.configCache["global"] = level
	return types.CompatibilityLevel(level), nil
}

// SetCompatibilityLevel sets the compatibility level for a subject
func (r *Registry) SetCompatibilityLevel(subject string, level types.CompatibilityLevel) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Validate compatibility level
	switch level {
	case types.Backward, types.Forward, types.Full, types.None, types.BackwardTransitive, types.ForwardTransitive, types.FullTransitive:
		// Valid
	default:
		return fmt.Errorf("invalid compatibility level: %s", level)
	}

	var key string
	if subject == "global" {
		key = keyPrefixGlobalConfig
	} else {
		key = keyPrefixSubjectConfig + subject
	}

	_, err := r.kvConfig.Put(key, []byte(level))
	return err
}

// CheckCompatibility checks if a new schema is compatible with an existing schema
func (r *Registry) CheckCompatibility(subject string, newSchema string, schemaType types.SchemaType, level types.CompatibilityLevel) (bool, error) {
	format, ok := r.formats[schemaType]
	if !ok {
		return false, fmt.Errorf("unsupported schema type: %s", schemaType)
	}

	// Get all versions for the subject
	versions, err := r.GetVersions(subject)
	if err != nil {
		if err.Error() == "no versions found" {
			// No existing schema, so any schema is compatible
			return true, nil
		}
		return false, err
	}

	// Sort versions to ensure we check in order
	sort.Ints(versions)

	// For transitive compatibility, we need to check against all previous versions
	if level == types.BackwardTransitive || level == types.ForwardTransitive || level == types.FullTransitive {
		for _, version := range versions {
			schema, err := r.getSchemaByVersion(subject, version)
			if err != nil {
				return false, err
			}

			// Check compatibility with this version
			compatible, err := format.CheckCompatibility(schema.Schema, newSchema, level)
			if err != nil {
				return false, err
			}
			if !compatible {
				return false, nil
			}
		}
		return true, nil
	}

	// For non-transitive compatibility, only check against the latest version
	latestVersion := versions[len(versions)-1]
	schema, err := r.getSchemaByVersion(subject, latestVersion)
	if err != nil {
		return false, err
	}

	// Check compatibility
	return format.CheckCompatibility(schema.Schema, newSchema, level)
}

// Serialize serializes data according to a schema
func (r *Registry) Serialize(data interface{}, schemaID int) ([]byte, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Get the schema by ID
	schema, err := r.GetSchema(schemaID)
	if err != nil {
		return nil, fmt.Errorf("get schema: %w", err)
	}

	format, ok := r.formats[schema.Type]
	if !ok {
		return nil, fmt.Errorf("unsupported schema type: %s", schema.Type)
	}

	// Serialize data
	serialized, err := format.Serialize(data, schema.Schema)
	if err != nil {
		return nil, fmt.Errorf("serialize: %w", err)
	}

	// Create wire format with magic byte and schema ID
	wireFormat := WireFormat{
		MagicByte: MagicByte,
		SchemaID:  int32(schemaID),
		Data:      serialized,
	}

	// Combine into final bytes
	result := make([]byte, 5+len(serialized))
	result[0] = wireFormat.MagicByte
	// Schema ID as 4 bytes in big-endian format
	result[1] = byte(wireFormat.SchemaID >> 24)
	result[2] = byte(wireFormat.SchemaID >> 16)
	result[3] = byte(wireFormat.SchemaID >> 8)
	result[4] = byte(wireFormat.SchemaID)
	copy(result[5:], wireFormat.Data)

	return result, nil
}

// Deserialize deserializes data according to a schema
func (r *Registry) Deserialize(data []byte) (interface{}, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Parse wire format
	if len(data) < 5 {
		return nil, fmt.Errorf("data too short")
	}

	// Verify magic byte
	if data[0] != MagicByte {
		return nil, fmt.Errorf("invalid magic byte")
	}

	// Extract schema ID
	schemaID := int(data[1])<<24 | int(data[2])<<16 | int(data[3])<<8 | int(data[4])
	payload := data[5:]

	// Get the schema by ID
	schema, err := r.GetSchema(schemaID)
	if err != nil {
		return nil, fmt.Errorf("get schema: %w", err)
	}

	format, ok := r.formats[schema.Type]
	if !ok {
		return nil, fmt.Errorf("unsupported schema type: %s", schema.Type)
	}

	// Deserialize data
	return format.Deserialize(payload, schema.Schema)
}

// GetSchemaById is an alias for GetSchema to match the API naming
func (r *Registry) GetSchemaById(id string) (*types.Schema, error) {
	idNum, err := strconv.Atoi(id)
	if err != nil {
		return nil, fmt.Errorf("invalid schema ID: %s", id)
	}
	return r.GetSchema(idNum)
}

// DeleteSchemaVersion deletes a specific version of a schema
func (r *Registry) DeleteSchemaVersion(subject string, version string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var versionNum int
	var err error

	// Handle "latest" version
	if version == "latest" {
		versionNum, err = r.getLatestVersion(subject)
		if err != nil {
			return err
		}
	} else {
		versionNum, err = strconv.Atoi(version)
		if err != nil {
			return fmt.Errorf("invalid version: %s", version)
		}
	}

	// Check if version exists
	key := fmt.Sprintf("%s%s/versions/%d", keyPrefixSubjects, subject, versionNum)
	if _, err := r.kvSchemas.Get(key); err != nil {
		return fmt.Errorf("version not found")
	}

	// Delete the version
	if err := r.kvSchemas.Delete(key); err != nil {
		return fmt.Errorf("delete version: %w", err)
	}

	return nil
}

// DeleteSubject deletes all versions of a subject
func (r *Registry) DeleteSubject(subject string) ([]int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Get all versions
	versions, err := r.GetVersions(subject)
	if err != nil {
		return nil, err
	}

	if len(versions) == 0 {
		return nil, fmt.Errorf("subject not found")
	}

	slog.Debug("DeleteSubject: versions to delete", "subject", subject, "versions", versions)
	deletedIDs := make([]int, 0, len(versions))
	for _, version := range versions {
		key := fmt.Sprintf("%s%s/versions/%d", keyPrefixSubjects, subject, version)
		// Get schema before deleting
		entry, err := r.kvSchemas.Get(key)
		if err == nil {
			var schema types.Schema
			if err := json.Unmarshal(entry.Value(), &schema); err == nil {
				slog.Debug("DeleteSubject: deleting schema version", "version", version, "id", schema.ID)
				deletedIDs = append(deletedIDs, schema.ID)
				// Delete schema by ID if it exists
				schemaKey := keyPrefixSchemas + strconv.Itoa(schema.ID)
				if err := r.kvSchemas.Delete(schemaKey); err != nil {
					slog.Debug("DeleteSubject: failed to delete schema by ID", "id", schema.ID, "err", err)
				} else {
					slog.Debug("DeleteSubject: deleted schema by ID", "id", schema.ID)
				}
				delete(r.schemaCache, schema.ID)
			}
		}
		if err := r.kvSchemas.Delete(key); err != nil {
			slog.Debug("DeleteSubject: failed to delete version key", "key", key, "err", err)
			return nil, fmt.Errorf("delete version %d: %w", version, err)
		}
	}

	// Remove from cache
	delete(r.subjectCache, subject)
	delete(r.versionCache, subject)

	slog.Debug("DeleteSubject: deleted IDs", "ids", deletedIDs)
	return deletedIDs, nil
}

// LookupSchema checks if a schema is already registered under a subject
func (r *Registry) LookupSchema(subject string, schemaStr string, schemaType types.SchemaType) (*types.Schema, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Validate schema format
	format, ok := r.formats[schemaType]
	if !ok {
		return nil, fmt.Errorf("unsupported schema type: %s", schemaType)
	}

	// Validate the schema
	if err := format.Validate(schemaStr); err != nil {
		return nil, fmt.Errorf("validate schema: %w", err)
	}

	// Get all versions
	versions, err := r.GetVersions(subject)
	if err != nil {
		return nil, err
	}

	// Check each version
	for _, version := range versions {
		schema, err := r.getSchemaByVersion(subject, version)
		if err != nil {
			continue
		}

		slog.Debug("Checking schema", "subject", subject, "version", version, "schema", schema.Schema, "schemaType", schema.Type)
		slog.Debug("Schema", "schema", schema.Schema, "schemaType", schema.Type)
		// Check if schemas are equal
		if schema.Schema == schemaStr && schema.Type == schemaType {
			return schema, nil
		} else {
			slog.Debug("Schema mismatch", "subject", subject, "version", version, "schema", schema.Schema, "schemaType", schema.Type)
		}
	}

	return nil, fmt.Errorf("schema not found")
}
