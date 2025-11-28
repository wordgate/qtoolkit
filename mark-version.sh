#!/bin/bash

# mark-version.sh - Batch tag all modules in qtoolkit
# Usage: ./mark-version.sh <version>
# Example: ./mark-version.sh v1.0.0

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Ensure we're in the qtoolkit root directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Discover all modules by finding go.mod files (excluding root)
MODULES=()
while IFS= read -r dir; do
    if [ -n "$dir" ] && [ "$dir" != "." ]; then
        MODULES+=("$dir")
    fi
done < <(find . -name "go.mod" -type f -exec dirname {} \; | sed 's|^\./||' | sort)

if [ ${#MODULES[@]} -eq 0 ]; then
    echo -e "${RED}Error: No modules found${NC}"
    exit 1
fi

# Check arguments
if [ -z "$1" ]; then
    echo -e "${RED}Error: Version number required${NC}"
    echo "Usage: $0 <version>"
    echo "Example: $0 v1.0.0"
    exit 1
fi

VERSION="$1"

# Validate version format (vX.Y.Z)
if ! [[ "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo -e "${RED}Error: Invalid version format${NC}"
    echo "Version must be in format: vX.Y.Z (e.g., v1.0.0)"
    exit 1
fi

# Check if it's a git repository
if ! git rev-parse --git-dir > /dev/null 2>&1; then
    echo -e "${RED}Error: Not a git repository${NC}"
    exit 1
fi

# Check for uncommitted changes
if ! git diff-index --quiet HEAD -- 2>/dev/null; then
    echo -e "${RED}Error: You have uncommitted changes${NC}"
    echo "Please commit or stash your changes before tagging."
    git status --short
    exit 1
fi

echo -e "${GREEN}Tagging all modules with version: ${VERSION}${NC}"
echo "Found ${#MODULES[@]} modules:"
printf '  %s\n' "${MODULES[@]}"
echo "================================================"

CREATED_TAGS=()

# Create root tag
if git rev-parse "$VERSION" >/dev/null 2>&1; then
    echo -e "${YELLOW}[SKIP] ${VERSION} (exists)${NC}"
else
    git tag "$VERSION"
    CREATED_TAGS+=("$VERSION")
    echo -e "${GREEN}[TAG] ${VERSION}${NC}"
fi

# Create module tags
for module in "${MODULES[@]}"; do
    tag="${module}/${VERSION}"

    if git rev-parse "$tag" >/dev/null 2>&1; then
        echo -e "${YELLOW}[SKIP] ${tag} (exists)${NC}"
    else
        git tag "$tag"
        CREATED_TAGS+=("$tag")
        echo -e "${GREEN}[TAG] ${tag}${NC}"
    fi
done

# Push all tags at once
if [ ${#CREATED_TAGS[@]} -gt 0 ]; then
    echo "================================================"
    echo "Pushing ${#CREATED_TAGS[@]} tags..."
    git push origin "${CREATED_TAGS[@]}"
    echo -e "${GREEN}Done${NC}"
else
    echo "================================================"
    echo -e "${YELLOW}No new tags to push${NC}"
fi
