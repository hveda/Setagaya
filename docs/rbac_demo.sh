#!/bin/bash

# RBAC System Testing and Demo Script
# This script demonstrates how to test the new RBAC functionality

echo "=== Shibuya RBAC System Demo ==="
echo

# Configuration
SHIBUYA_URL="http://localhost:8080"
API_BASE="$SHIBUYA_URL/api"

# Helper function for API calls
call_api() {
    local method=$1
    local endpoint=$2
    local data=$3
    local auth_header=$4
    
    if [ -n "$data" ]; then
        if [ -n "$auth_header" ]; then
            curl -s -X "$method" \
                 -H "Content-Type: application/json" \
                 -H "$auth_header" \
                 -d "$data" \
                 "$API_BASE$endpoint"
        else
            curl -s -X "$method" \
                 -H "Content-Type: application/json" \
                 -d "$data" \
                 "$API_BASE$endpoint"
        fi
    else
        if [ -n "$auth_header" ]; then
            curl -s -X "$method" \
                 -H "$auth_header" \
                 "$API_BASE$endpoint"
        else
            curl -s -X "$method" \
                 "$API_BASE$endpoint"
        fi
    fi
}

echo "1. Testing Role Management"
echo "=========================="

echo "Getting all roles:"
call_api "GET" "/rbac/roles" | jq '.'
echo

echo "Creating a custom role:"
call_api "POST" "/rbac/roles" '{
    "name": "test_manager", 
    "description": "Test manager role for demo"
}' | jq '.'
echo

echo "2. Testing Permission Management"
echo "==============================="

echo "Getting all permissions:"
call_api "GET" "/rbac/permissions" | jq '.' | head -20
echo "... (truncated for brevity)"
echo

echo "3. Testing User Management"
echo "========================="

echo "Getting all users:"
call_api "GET" "/rbac/users" | jq '.'
echo

echo "Creating a test user:"
call_api "POST" "/rbac/users" '{
    "username": "testuser",
    "email": "test@example.com",
    "full_name": "Test User"
}' | jq '.'
echo

echo "4. Testing Current User Info"
echo "==========================="

echo "Getting current user RBAC info:"
call_api "GET" "/rbac/me" | jq '.'
echo

echo "5. Database Schema Verification"
echo "==============================="

echo "Checking if RBAC tables exist:"
echo "This would typically be done with database tools:"
echo "mysql -u [user] -p shibuya -e \"SHOW TABLES LIKE '%role%';\""
echo "mysql -u [user] -p shibuya -e \"SHOW TABLES LIKE '%permission%';\""
echo "mysql -u [user] -p shibuya -e \"SHOW TABLES LIKE '%user%';\""
echo

echo "6. Testing Permission Checks"
echo "============================"

echo "Examples of permission checking in code:"
echo ""
echo "// Check if user has specific permission"
echo "hasPermission, err := model.HasPermission(\"username\", \"projects:create\")"
echo "if err != nil {"
echo "    return err"
echo "}"
echo "if !hasPermission {"
echo "    return errors.New(\"insufficient permissions\")"
echo "}"
echo ""
echo "// Check resource-specific permission"
echo "hasResourcePermission, err := model.HasResourcePermission(\"username\", \"projects\", \"read\")"
echo

echo "7. Middleware Usage Examples"
echo "============================"

echo "Examples of using RBAC middleware:"
echo ""
echo "// Require specific permission"
echo "r.HandlerFunc = s.requirePermission(\"projects:create\")(handler)"
echo ""
echo "// Require specific role"
echo "r.HandlerFunc = s.requireRole(\"administrator\")(handler)"
echo ""
echo "// Require admin access"
echo "r.HandlerFunc = s.requireAdminRole(handler)"
echo ""
echo "// Use RBAC with ownership checking"
echo "r.HandlerFunc = s.projectOwnershipRequired(handler)"
echo

echo "=== Demo Complete ==="
echo
echo "To use this RBAC system:"
echo "1. Apply the database migration: SOURCE shibuya/db/20241201.sql;"
echo "2. Start the Shibuya server"
echo "3. Use the API endpoints to manage roles and users"
echo "4. Existing project ownership will continue to work"
echo "5. New users will automatically get 'loadtest_user' role"
echo
echo "See docs/RBAC.md for complete documentation."