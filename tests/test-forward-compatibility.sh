#!/bin/bash
# Schema Registry Forward Compatibility Test Script for macOS with command echoing

# Schema Registry URL - adjust as needed
SCHEMA_REGISTRY_URL="http://localhost:8081"

# Function to execute a command and echo it first
run_command() {
  echo -e "\n> $@"
  eval "$@"
}

# Exit on error
set -e

echo "=== Schema Registry FORWARD Compatibility Test ==="
echo

# 1. Check Schema Registry is running
echo "Checking Schema Registry availability..."
run_command "curl -s $SCHEMA_REGISTRY_URL > /dev/null || { echo \"Schema Registry not available at $SCHEMA_REGISTRY_URL\"; exit 1; }"
echo "Schema Registry is available!"
echo

# 2. Set global compatibility mode to FORWARD
echo "Setting global compatibility mode to FORWARD..."
run_command "curl -X PUT -H \"Content-Type: application/vnd.schemaregistry.v1+json\" \
  --data '{\"compatibility\": \"FORWARD\"}' \
  $SCHEMA_REGISTRY_URL/config"
echo -e "\nGlobal compatibility set to FORWARD"
echo

# 3. Create test subject
TEST_SUBJECT="user-forward-test"
echo "Using test subject: $TEST_SUBJECT"
echo

# 4. Register initial schema (v1) - simpler schema that will be evolved
echo "Registering initial schema (v1)..."
V1_SCHEMA='{"schema":"{\"type\":\"record\",\"name\":\"User\",\"namespace\":\"com.example\",\"fields\":[{\"name\":\"id\",\"type\":\"int\"},{\"name\":\"name\",\"type\":\"string\"}]}"}'
run_command "curl -X POST -H \"Content-Type: application/vnd.schemaregistry.v1+json\" \
  --data '$V1_SCHEMA' \
  $SCHEMA_REGISTRY_URL/subjects/$TEST_SUBJECT/versions"

echo -e "\nInitial schema registered!"
echo

# 5. Try to register an incompatible schema (adding a required field)
echo "Trying to register incompatible schema (adding required 'email' field)..."
V2_INCOMPATIBLE_SCHEMA='{"schema":"{\"type\":\"record\",\"name\":\"User\",\"namespace\":\"com.example\",\"fields\":[{\"name\":\"id\",\"type\":\"int\"},{\"name\":\"name\",\"type\":\"string\"},{\"name\":\"email\",\"type\":\"string\"}]}"}'

run_command "INCOMPATIBLE_RESPONSE=\$(curl -s -w \"%{http_code}\" -X POST -H \"Content-Type: application/vnd.schemaregistry.v1+json\" \
  --data '$V2_INCOMPATIBLE_SCHEMA' \
  $SCHEMA_REGISTRY_URL/subjects/$TEST_SUBJECT/versions)"

echo "Response received. Checking status code..."
run_command "HTTP_CODE=\$(echo \"\$INCOMPATIBLE_RESPONSE\" | tail -c 4)"
run_command "if [[ \"\$HTTP_CODE\" == \"409\" ]]; then
  echo \"Test PASSED: Incompatible schema correctly rejected\"
else
  echo \"Test FAILED: Incompatible schema was accepted\"
  echo \"Response: \$INCOMPATIBLE_RESPONSE\"
fi"
echo

# 6. Check compatibility explicitly
echo "Checking incompatible schema compatibility explicitly..."
run_command "curl -X POST -H \"Content-Type: application/vnd.schemaregistry.v1+json\" \
  --data '$V2_INCOMPATIBLE_SCHEMA' \
  $SCHEMA_REGISTRY_URL/compatibility/subjects/$TEST_SUBJECT/versions/latest"
echo
echo

# 7. Register a compatible schema (adding optional field with default)
echo "Registering compatible schema (adding optional 'email' field with default)..."
V2_COMPATIBLE_SCHEMA='{"schema":"{\"type\":\"record\",\"name\":\"User\",\"namespace\":\"com.example\",\"fields\":[{\"name\":\"id\",\"type\":\"int\"},{\"name\":\"name\",\"type\":\"string\"},{\"name\":\"email\",\"type\":[\"null\",\"string\"],\"default\":null}]}"}'

run_command "COMPATIBLE_RESPONSE=\$(curl -s -w \"%{http_code}\" -X POST -H \"Content-Type: application/vnd.schemaregistry.v1+json\" \
  --data '$V2_COMPATIBLE_SCHEMA' \
  $SCHEMA_REGISTRY_URL/subjects/$TEST_SUBJECT/versions)"

echo "Response received. Checking status code..."
run_command "HTTP_CODE=\$(echo \"\$COMPATIBLE_RESPONSE\" | tail -c 4)"
run_command "if [[ \"\$HTTP_CODE\" == \"200\" ]]; then
  echo \"Test PASSED: Compatible schema correctly accepted\"
else
  echo \"Test FAILED: Compatible schema was rejected\"
  echo \"Response: \$COMPATIBLE_RESPONSE\"
fi"
echo

# 8. Add another field with default (still forward compatible)
echo "Registering schema with another optional field (with default)..."
V3_SCHEMA='{"schema":"{\"type\":\"record\",\"name\":\"User\",\"namespace\":\"com.example\",\"fields\":[{\"name\":\"id\",\"type\":\"int\"},{\"name\":\"name\",\"type\":\"string\"},{\"name\":\"email\",\"type\":[\"null\",\"string\"],\"default\":null},{\"name\":\"address\",\"type\":[\"null\",\"string\"],\"default\":null}]}"}'

run_command "FIELD_RESPONSE=\$(curl -s -w \"%{http_code}\" -X POST -H \"Content-Type: application/vnd.schemaregistry.v1+json\" \
  --data '$V3_SCHEMA' \
  $SCHEMA_REGISTRY_URL/subjects/$TEST_SUBJECT/versions)"

echo "Response received. Checking status code..."
run_command "HTTP_CODE=\$(echo \"\$FIELD_RESPONSE\" | tail -c 4)"
run_command "if [[ \"\$HTTP_CODE\" == \"200\" ]]; then
  echo \"Test PASSED: Schema with another optional field correctly accepted\"
else
  echo \"Test FAILED: Schema with another optional field was rejected\"
  echo \"Response: \$FIELD_RESPONSE\"
fi"
echo

# 9. Check versions registered
echo "Checking all versions registered for $TEST_SUBJECT:"
run_command "curl -s $SCHEMA_REGISTRY_URL/subjects/$TEST_SUBJECT/versions"
echo
echo

# 10. Set subject-specific compatibility
echo "Setting subject-specific compatibility to FORWARD_TRANSITIVE..."
run_command "curl -X PUT -H \"Content-Type: application/vnd.schemaregistry.v1+json\" \
  --data '{\"compatibility\": \"FORWARD_TRANSITIVE\"}' \
  $SCHEMA_REGISTRY_URL/config/$TEST_SUBJECT"
echo -e "\nSubject compatibility set to FORWARD_TRANSITIVE"
echo

# 11. Try to register a schema that's compatible with latest but not all previous
echo "Registering a new schema that changes a field type (may be compatible with latest but not all)..."
V4_SCHEMA='{"schema":"{\"type\":\"record\",\"name\":\"User\",\"namespace\":\"com.example\",\"fields\":[{\"name\":\"id\",\"type\":\"long\"},{\"name\":\"name\",\"type\":\"string\"},{\"name\":\"email\",\"type\":[\"null\",\"string\"],\"default\":null},{\"name\":\"address\",\"type\":[\"null\",\"string\"],\"default\":null}]}"}'

# First check against latest only
echo "Checking compatibility with just the latest version:"
run_command "curl -X POST -H \"Content-Type: application/vnd.schemaregistry.v1+json\" \
  --data '$V4_SCHEMA' \
  $SCHEMA_REGISTRY_URL/compatibility/subjects/$TEST_SUBJECT/versions/latest"
echo

# Then check against all versions (for FORWARD_TRANSITIVE)
echo "Checking compatibility with all previous versions:"
run_command "curl -X POST -H \"Content-Type: application/vnd.schemaregistry.v1+json\" \
  --data '$V4_SCHEMA' \
  $SCHEMA_REGISTRY_URL/compatibility/subjects/$TEST_SUBJECT/versions"
echo

# 12. Try to register schema with field type change
run_command "TYPE_CHANGE_RESPONSE=\$(curl -s -w \"%{http_code}\" -X POST -H \"Content-Type: application/vnd.schemaregistry.v1+json\" \
  --data '$V4_SCHEMA' \
  $SCHEMA_REGISTRY_URL/subjects/$TEST_SUBJECT/versions)"

echo "Response received. Checking status code..."
run_command "HTTP_CODE=\$(echo \"\$TYPE_CHANGE_RESPONSE\" | tail -c 4)"
run_command "if [[ \"\$HTTP_CODE\" == \"409\" ]]; then
  echo \"Test as expected: Schema with type change rejected under FORWARD_TRANSITIVE mode\"
else
  echo \"Unexpected result: Schema with type change was accepted\"
  echo \"Response: \$TYPE_CHANGE_RESPONSE\"
fi"
echo

# 13. Try a schema that should be compatible with all versions
echo "Registering a new schema that's compatible with all versions..."
V5_SCHEMA='{"schema":"{\"type\":\"record\",\"name\":\"User\",\"namespace\":\"com.example\",\"fields\":[{\"name\":\"id\",\"type\":\"int\"},{\"name\":\"name\",\"type\":\"string\"},{\"name\":\"email\",\"type\":[\"null\",\"string\"],\"default\":null},{\"name\":\"address\",\"type\":[\"null\",\"string\"],\"default\":null},{\"name\":\"phone\",\"type\":[\"null\",\"string\"],\"default\":null}]}"}'

run_command "curl -X POST -H \"Content-Type: application/vnd.schemaregistry.v1+json\" \
  --data '$V5_SCHEMA' \
  $SCHEMA_REGISTRY_URL/compatibility/subjects/$TEST_SUBJECT/versions"
echo

run_command "COMPATIBLE_ALL_RESPONSE=\$(curl -s -w \"%{http_code}\" -X POST -H \"Content-Type: application/vnd.schemaregistry.v1+json\" \
  --data '$V5_SCHEMA' \
  $SCHEMA_REGISTRY_URL/subjects/$TEST_SUBJECT/versions)"

echo "Response received. Checking status code..."
run_command "HTTP_CODE=\$(echo \"\$COMPATIBLE_ALL_RESPONSE\" | tail -c 4)"
run_command "if [[ \"\$HTTP_CODE\" == \"200\" ]]; then
  echo \"Test PASSED: Schema compatible with all versions was accepted\"
else
  echo \"Test FAILED: Schema compatible with all versions was rejected\"
  echo \"Response: \$COMPATIBLE_ALL_RESPONSE\"
fi"
echo

# 14. Show final registered versions
echo "Checking final versions registered for $TEST_SUBJECT:"
run_command "curl -s $SCHEMA_REGISTRY_URL/subjects/$TEST_SUBJECT/versions"
echo
echo

# 15. Clean up (optional) - delete the test subject
echo "Cleaning up - deleting test subject..."
echo "> # curl -X DELETE $SCHEMA_REGISTRY_URL/subjects/$TEST_SUBJECT"
echo "NOTE: Uncomment the line above to actually delete the subject"
echo

echo "=== Test Completed ==="