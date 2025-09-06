#!/bin/bash

# Setup script for development tools
# Installs formatters and linters for consistent code quality

set -e

echo "🚀 Setting up Setagaya development tools..."

# Check if we have the required package managers
if ! command -v npm >/dev/null 2>&1; then
    echo "❌ npm is required but not installed. Install Node.js first."
    echo "   Visit: https://nodejs.org/"
    exit 1
fi

# Install Node.js dependencies
echo "📦 Installing Node.js dependencies..."
npm install

# Install system dependencies
echo "🔧 Installing system dependencies..."

# Check and install yamllint
if ! command -v yamllint >/dev/null 2>&1; then
    echo "Installing yamllint..."
    if command -v brew >/dev/null 2>&1; then
        brew install yamllint
    elif command -v pip3 >/dev/null 2>&1; then
        pip3 install yamllint
    elif command -v pip >/dev/null 2>&1; then
        pip install yamllint
    else
        echo "⚠️  Please install yamllint manually:"
        echo "   brew install yamllint"
        echo "   # or"
        echo "   pip install yamllint"
    fi
else
    echo "✅ yamllint already installed"
fi

# Install goimports for Go formatting
if command -v go >/dev/null 2>&1; then
    echo "Installing goimports..."
    go install golang.org/x/tools/cmd/goimports@latest
else
    echo "⚠️  Go not found. goimports will not be available."
fi

# Make the git hook executable
chmod +x .git/hooks/pre-commit

echo ""
echo "✅ Setup complete!"
echo ""
echo "Available commands:"
echo "  npm run format       # Format all files with prettier"
echo "  npm run lint:yaml    # Lint YAML files"
echo "  npm run lint:md      # Lint Markdown files"
echo "  npm run fix          # Auto-fix all issues"
echo ""
echo "The pre-commit hook will automatically format files on commit."
echo ""
echo "To manually run formatting:"
echo "  .git/hooks/pre-commit  # Test the pre-commit hook"
echo "  npm run fix           # Fix all formatting issues"
echo ""
