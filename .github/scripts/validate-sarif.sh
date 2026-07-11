#!/bin/bash

# GitHub Security Scanning SARIF Validation Script
# This script validates and enhances SARIF files for better GitHub Security tab integration

set -e

validate_sarif() {
    local sarif_file="$1"
    local tool_name="$2"
    local output_file="$3"

    echo "🔍 Validating SARIF file: $sarif_file"

    if [ ! -f "$sarif_file" ]; then
        echo "⚠️  SARIF file not found: $sarif_file"
        create_empty_sarif "$tool_name" "$output_file"
        return 1
    fi

    # Check if file is valid JSON
    if ! jq . "$sarif_file" > /dev/null 2>&1; then
        echo "❌ Invalid JSON in SARIF file: $sarif_file"
        create_empty_sarif "$tool_name" "$output_file"
        return 1
    fi

    # Check if it has the required SARIF structure
    if ! jq -e '.version and .runs' "$sarif_file" > /dev/null 2>&1; then
        echo "❌ Invalid SARIF structure in: $sarif_file"
        create_empty_sarif "$tool_name" "$output_file"
        return 1
    fi

    # Enhance SARIF with required fields
    jq --arg tool_name "$tool_name" '
        .version = "2.1.0" |
        ."$schema" = "https://raw.githubusercontent.com/oasis-tcs/sarif-spec/master/Schemata/sarif-schema-2.1.0.json" |
        .runs[] |= (
            .tool.driver.name = $tool_name |
            if .tool.driver.informationUri then . else .tool.driver.informationUri = "https://github.com/heridotlife/Setagaya" end |
            .results[]? |= (
                if .locations[]?.physicalLocation.artifactLocation then . 
                else (.locations[]?.physicalLocation.artifactLocation = {"uri": "."}) end
            )
        )
    ' "$sarif_file" > "$output_file"

    echo "✅ SARIF validation completed: $output_file"
    return 0
}

create_empty_sarif() {
    local tool_name="$1"
    local output_file="$2"

    echo "🔧 Creating empty SARIF file for: $tool_name"

    cat > "$output_file" << EOF
{
  "version": "2.1.0",
  "\$schema": "https://raw.githubusercontent.com/oasis-tcs/sarif-spec/master/Schemata/sarif-schema-2.1.0.json",
  "runs": [
    {
      "tool": {
        "driver": {
          "name": "$tool_name",
          "version": "1.0.0",
          "informationUri": "https://github.com/heridotlife/Setagaya"
        }
      },
      "results": []
    }
  ]
}
EOF
}

generate_summary() {
    local sarif_file="$1"
    local tool_name="$2"

    if [ ! -f "$sarif_file" ]; then
        echo "❌ **$tool_name**: No results file generated"
        return 1
    fi

    if ! jq . "$sarif_file" > /dev/null 2>&1; then
        echo "❌ **$tool_name**: Invalid results format"
        return 1
    fi

    local total_results=$(jq -r '[.runs[].results[]] | length' "$sarif_file" 2>/dev/null || echo "0")
    local high_results=$(jq -r '[.runs[].results[] | select(.level == "error")] | length' "$sarif_file" 2>/dev/null || echo "0")
    local medium_results=$(jq -r '[.runs[].results[] | select(.level == "warning")] | length' "$sarif_file" 2>/dev/null || echo "0")
    local low_results=$(jq -r '[.runs[].results[] | select(.level == "note")] | length' "$sarif_file" 2>/dev/null || echo "0")

    echo "### 🔍 $tool_name Results"
    echo "- 🔴 High/Critical: $high_results"
    echo "- 🟡 Medium: $medium_results"
    echo "- ℹ️ Low/Info: $low_results"
    echo "- 📊 Total: $total_results"

    if [ "$high_results" -gt "0" ]; then
        echo "⚠️ **Action Required**: High severity issues detected"
        return 2
    elif [ "$total_results" -eq "0" ]; then
        echo "✅ **Status**: No issues detected"
        return 0
    else
        echo "✅ **Status**: Issues detected but no high severity"
        return 0
    fi
}

# Main execution
if [ $# -lt 3 ]; then
    echo "Usage: $0 <input_sarif> <tool_name> <output_sarif> [generate_summary]"
    echo "Example: $0 grype-results.sarif Grype grype-validated.sarif true"
    exit 1
fi

INPUT_SARIF="$1"
TOOL_NAME="$2"
OUTPUT_SARIF="$3"
GENERATE_SUMMARY="${4:-false}"

# Validate and fix SARIF
validate_sarif "$INPUT_SARIF" "$TOOL_NAME" "$OUTPUT_SARIF"
validation_result=$?

# Generate summary if requested
if [ "$GENERATE_SUMMARY" = "true" ]; then
    echo ""
    generate_summary "$OUTPUT_SARIF" "$TOOL_NAME"
fi

exit $validation_result