#!/bin/bash
# Schema Registry FULL Compatibility Test Script for macOS with command echoing

# Schema Registry URL - adjust as needed
SCHEMA_REGISTRY_URL="http://localhost:8081"

# Function to execute a command and echo it first
run_command() {
  echo -e "\n> $@"
  eval "$@"
}

# Exit on error
set -e

echo "=== Schema Registry FULL Compatibility Test ==="
echo
echo "FULL compatibility means schemas must be both FORWARD and BACKWARD compatible."
echo "This ensures consumers using older schemas can read data produced with new schemas"
echo "AND consumers using newer schemas can read data produced with older schemas."
echo

# 1. Check Schema Registry is running
echo "Checking Schema Registry availability..."
run_command "curl -s $SCHEMA_REGISTRY_URL > /dev/null || { echo \"Schema Registry not available at $SCHEMA_REGISTRY_URL\"; exit 1; }"
echo "Schema Registry is available!"
echo

# 2. Set global compatibility mode to FULL
echo "Setting global compatibility mode to FULL..."
run_command "curl -X PUT -H \"Content-Type: application/vnd.schemaregistry.v1+json\" \
  --data '{\"compatibility\": \"FULL\"}' \
  $SCHEMA_REGISTRY_URL/config"
echo -e "\nGlobal compatibility set to FULL"
echo

# 3. Create test subject
TEST_SUBJECT="user-full-test"
echo "Using test subject: $TEST_SUBJECT"
echo

# 4. Register initial schema (v1)
echo "Registering initial schema (v1)..."
V1_SCHEMA='{"schema":"{\"type\":\"record\",\"name\":\"User\",\"namespace\":\"com.example\",\"fields\":[{\"name\":\"id\",\"type\":\"int\"},{\"name\":\"name\",\"type\":\"string\"}]}"}'
run_command "curl -X POST -H \"Content-Type: application/vnd.schemaregistry.v1+json\" \
  --data '$V1_SCHEMA' \
  $SCHEMA_REGISTRY_URL/subjects/$TEST_SUBJECT/versions"

echo -e "\nInitial schema registered!"
echo

# 5. Try to register an incompatible schema (adding a required field)
echo "TEST 1: Trying to register BACKWARD incompatible schema (adding required 'email' field)..."
V2_BACKWARD_INCOMPATIBLE='{"schema":"{\"type\":\"record\",\"name\":\"User\",\"namespace\":\"com.example\",\"fields\":[{\"name\":\"id\",\"type\":\"int\"},{\"name\":\"name\",\"type\":\"string\"},{\"name\":\"email\",\"type\":\"string\"}]}"}'

run_command "INCOMPATIBLE_RESPONSE=\$(curl -s -w \"%{http_code}\" -X POST -H \"Content-Type: application/vnd.schemaregistry.v1+json\" \
  --data '$V2_BACKWARD_INCOMPATIBLE' \
  $SCHEMA_REGISTRY_URL/subjects/$TEST_SUBJECT/versions)"

echo "Response received. Checking status code..."
run_command "HTTP_CODE=\$(echo \"\$INCOMPATIBLE_RESPONSE\" | tail -c 4)"
run_command "if [[ \"\$HTTP_CODE\" == \"409\" ]]; then
  echo \"Test PASSED: Backward incompatible schema correctly rejected\"
else
  echo \"Test FAILED: Backward incompatible schema was accepted\"
  echo \"Response: \$INCOMPATIBLE_RESPONSE\"
fi"
echo

# 6. Try to register a different incompatible schema (removing a field without default)
echo "TEST 2: Trying to register FORWARD incompatible schema (removing 'name' field)..."
V2_FORWARD_INCOMPATIBLE='{"schema":"{\"type\":\"record\",\"name\":\"User\",\"namespace\":\"com.example\",\"fields\":[{\"name\":\"id\",\"type\":\"int\"}]}"}'

run_command "INCOMPATIBLE_RESPONSE2=\$(curl -s -w \"%{http_code}\" -X POST -H \"Content-Type: application/vnd.schemaregistry.v1+json\" \
  --data '$V2_FORWARD_INCOMPATIBLE' \
  $SCHEMA_REGISTRY_URL/subjects/$TEST_SUBJECT/versions)"

echo "Response received. Checking status code..."
run_command "HTTP_CODE=\$(echo \"\$INCOMPATIBLE_RESPONSE2\" | tail -c 4)"
run_command "if [[ \"\$HTTP_CODE\" == \"409\" ]]; then
  echo \"Test PASSED: Forward incompatible schema correctly rejected\"
else
  echo \"Test FAILED: Forward incompatible schema was accepted\"
  echo \"Response: \$INCOMPATIBLE_RESPONSE2\"
fi"
echo

# 7. Check compatibility explicitly for both incompatible schemas
echo "Checking compatibility of BACKWARD incompatible schema explicitly..."
run_command "curl -X POST -H \"Content-Type: application/vnd.schemaregistry.v1+json\" \
  --data '$V2_BACKWARD_INCOMPATIBLE' \
  $SCHEMA_REGISTRY_URL/compatibility/subjects/$TEST_SUBJECT/versions/latest"
echo

echo "Checking compatibility of FORWARD incompatible schema explicitly..."
run_command "curl -X POST -H \"Content-Type: application/vnd.schemaregistry.v1+json\" \
  --data '$V2_FORWARD_INCOMPATIBLE' \
  $SCHEMA_REGISTRY_URL/compatibility/subjects/$TEST_SUBJECT/versions/latest"
echo
echo

# 8. Register a FULL compatible schema (adding optional field with default)
echo "TEST 3: Registering FULL compatible schema (adding optional 'email' field with default)..."
V2_FULL_COMPATIBLE='{"schema":"{\"type\":\"record\",\"name\":\"User\",\"namespace\":\"com.example\",\"fields\":[{\"name\":\"id\",\"type\":\"int\"},{\"name\":\"name\",\"type\":\"string\"},{\"name\":\"email\",\"type\":[\"null\",\"string\"],\"default\":null}]}"}'

run_command "COMPATIBLE_RESPONSE=\$(curl -s -w \"%{http_code}\" -X POST -H \"Content-Type: application/vnd.schemaregistry.v1+json\" \
  --data '$V2_FULL_COMPATIBLE' \
  $SCHEMA_REGISTRY_URL/subjects/$TEST_SUBJECT/versions)"

echo "Response received. Checking status code..."
run_command "HTTP_CODE=\$(echo \"\$COMPATIBLE_RESPONSE\" | tail -c 4)"
run_command "if [[ \"\$HTTP_CODE\" == \"200\" ]]; then
  echo \"Test PASSED: FULL compatible schema correctly accepted\"
else
  echo \"Test FAILED: FULL compatible schema was rejected\"
  echo \"Response: \$COMPATIBLE_RESPONSE\"
fi"
echo

# 9. Check versions registered so far
echo "Checking versions registered after adding compatible schema:"
run_command "curl -s $SCHEMA_REGISTRY_URL/subjects/$TEST_SUBJECT/versions"
echo
echo

# 10. Try type change (FULL compatibility has specific rules for type changes)
echo "TEST 4: Trying schema with type change (int to long for id)..."
V3_TYPE_CHANGE='{"schema":"{\"type\":\"record\",\"name\":\"User\",\"namespace\":\"com.example\",\"fields\":[{\"name\":\"id\",\"type\":\"long\"},{\"name\":\"name\",\"type\":\"string\"},{\"name\":\"email\",\"type\":[\"null\",\"string\"],\"default\":null}]}"}'

run_command "TYPE_RESPONSE=\$(curl -s -w \"%{http_code}\" -X POST -H \"Content-Type: application/vnd.schemaregistry.v1+json\" \
  --data '$V3_TYPE_CHANGE' \
  $SCHEMA_REGISTRY_URL/subjects/$TEST_SUBJECT/versions)"

echo "Response received. Checking status code..."
run_command "HTTP_CODE=\$(echo \"\$TYPE_RESPONSE\" | tail -c 4)"
run_command "if [[ \"\$HTTP_CODE\" == \"409\" ]]; then
  echo \"Test as expected: Schema with change from int to long rejected in FULL mode\"
  echo \"This is because old consumers might not handle the larger long values\"
else
  echo \"Unexpected result: Schema with type change was accepted\"
  echo \"Response: \$TYPE_RESPONSE\"
fi"
echo

# 11. Modify field order (should be compatible in FULL mode)
echo "TEST 5: Trying schema with field order change (should be compatible)..."
V3_FIELD_ORDER='{"schema":"{\"type\":\"record\",\"name\":\"User\",\"namespace\":\"com.example\",\"fields\":[{\"name\":\"name\",\"type\":\"string\"},{\"name\":\"id\",\"type\":\"int\"},{\"name\":\"email\",\"type\":[\"null\",\"string\"],\"default\":null}]}"}'

run_command "curl -X POST -H \"Content-Type: application/vnd.schemaregistry.v1+json\" \
  --data '$V3_FIELD_ORDER' \
  $SCHEMA_REGISTRY_URL/compatibility/subjects/$TEST_SUBJECT/versions/latest"
echo

run_command "ORDER_RESPONSE=\$(curl -s -w \"%{http_code}\" -X POST -H \"Content-Type: application/vnd.schemaregistry.v1+json\" \
  --data '$V3_FIELD_ORDER' \
  $SCHEMA_REGISTRY_URL/subjects/$TEST_SUBJECT/versions)"

echo "Response received. Checking status code..."
run_command "HTTP_CODE=\$(echo \"\$ORDER_RESPONSE\" | tail -c 4)"
run_command "if [[ \"\$HTTP_CODE\" == \"200\" ]]; then
  echo \"Test PASSED: Field order change correctly accepted\"
else
  echo \"Test FAILED: Field order change was rejected\"
  echo \"Response: \$ORDER_RESPONSE\"
fi"
echo

# 12. Set subject-specific compatibility to FULL_TRANSITIVE
echo "Setting subject-specific compatibility to FULL_TRANSITIVE..."
run_command "curl -X PUT -H \"Content-Type: application/vnd.schemaregistry.v1+json\" \
  --data '{\"compatibility\": \"FULL_TRANSITIVE\"}' \
  $SCHEMA_REGISTRY_URL/config/$TEST_SUBJECT"
echo -e "\nSubject compatibility set to FULL_TRANSITIVE"
echo

# 13. Add another field and check compatibility with all previous versions
echo "TEST 6: Adding another field and checking FULL_TRANSITIVE compatibility..."
V4_ADD_FIELD='{"schema":"{\"type\":\"record\",\"name\":\"User\",\"namespace\":\"com.example\",\"fields\":[{\"name\":\"name\",\"type\":\"string\"},{\"name\":\"id\",\"type\":\"int\"},{\"name\":\"email\",\"type\":[\"null\",\"string\"],\"default\":null},{\"name\":\"address\",\"type\":[\"null\",\"string\"],\"default\":null}]}"}'

echo "Checking compatibility with all previous versions (FULL_TRANSITIVE):"
run_command "curl -X POST -H \"Content-Type: application/vnd.schemaregistry.v1+json\" \
  --data '$V4_ADD_FIELD' \
  $SCHEMA_REGISTRY_URL/compatibility/subjects/$TEST_SUBJECT/versions"
echo

run_command "TRANSITIVE_RESPONSE=\$(curl -s -w \"%{http_code}\" -X POST -H \"Content-Type: application/vnd.schemaregistry.v1+json\" \
  --data '$V4_ADD_FIELD' \
  $SCHEMA_REGISTRY_URL/subjects/$TEST_SUBJECT/versions)"

echo "Response received. Checking status code..."
run_command "HTTP_CODE=\$(echo \"\$TRANSITIVE_RESPONSE\" | tail -c 4)"
run_command "if [[ \"\$HTTP_CODE\" == \"200\" ]]; then
  echo \"Test PASSED: FULL_TRANSITIVE compatible schema correctly accepted\"
else
  echo \"Test FAILED: FULL_TRANSITIVE compatible schema was rejected\"
  echo \"Response: \$TRANSITIVE_RESPONSE\"
fi"
echo

# 14. Show all registered versions
echo "Checking all final versions registered for $TEST_SUBJECT:"
run_command "curl -s $SCHEMA_REGISTRY_URL/subjects/$TEST_SUBJECT/versions"
echo
echo

# 15. Get the actual schemas for all versions
echo "Fetching the actual schemas for all versions:"
for version in 1 2 3 4; do
  echo "Schema version $version:"
  run_command "curl -s $SCHEMA_REGISTRY_URL/subjects/$TEST_SUBJECT/versions/$version"
  echo
done
echo

# 16. Clean up (optional) - delete the test subject
echo "Cleaning up - deleting test subject..."
echo "> # curl -X DELETE $SCHEMA_REGISTRY_URL/subjects/$TEST_SUBJECT"
echo "NOTE: Uncomment the line above to actually delete the subject"
echo

echo "=== Test Completed ==="
echo 
echo "SUMMARY OF FULL COMPATIBILITY RULES:"
echo "1. ✅ Adding optional fields with defaults is compatible"
echo "2. ❌ Adding required fields breaks backward compatibility"
echo "3. ❌ Removing fields breaks forward compatibility"
echo "4. ❌ Changing field types generally breaks compatibility (some exceptions exist)"
echo "5. ✅ Changing field order is allowed (Avro doesn't use field position)"
echo "6. FULL_TRANSITIVE enforces compatibility with ALL previous versions, not just the latest"