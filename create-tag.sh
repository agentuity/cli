#!/bin/bash

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}🚀 Git Tag Creator${NC}"
echo "=================="

# Check if we're on main branch
current_branch=$(git branch --show-current)
if [ "$current_branch" != "main" ]; then
    echo -e "${RED}❌ Error: You are not on the main branch. Current branch: $current_branch${NC}"
    echo "Please switch to main branch first:"
    echo "  git checkout main"
    exit 1
fi

echo -e "${GREEN}✅ On main branch${NC}"

# Check if main is up to date
echo "Checking if main is up to date..."
git fetch origin

behind=$(git rev-list --count HEAD..origin/main)
ahead=$(git rev-list --count origin/main..HEAD)

if [ "$behind" -gt 0 ]; then
    echo -e "${YELLOW}⚠️  Your local main is $behind commits behind origin/main${NC}"
    echo -e "${YELLOW}⚠️  Your local main is $ahead commits ahead of origin/main${NC}"
    
    read -p "Would you like to pull the latest changes? (y/N): " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        echo "Pulling latest changes..."
        git pull origin main
    else
        echo -e "${RED}❌ Stopping. You need to manually sync your branch because you're doing something weird.${NC}"
        echo "Run these commands manually:"
        echo "  git pull origin main"
        echo "  ./create-tag.sh"
        exit 1
    fi
elif [ "$ahead" -gt 0 ]; then
    echo -e "${YELLOW}⚠️  Your local main is $ahead commits ahead of origin/main${NC}"
    echo "This is unusual. You might want to push your changes first."
    read -p "Continue anyway? (y/N): " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo -e "${RED}❌ Stopping. Please push your changes first.${NC}"
        exit 1
    fi
else
    echo -e "${GREEN}✅ Main is up to date${NC}"
fi

# Get the latest tag
latest_tag=$(git describe --tags --abbrev=0 2>/dev/null || echo "")

if [ -z "$latest_tag" ]; then
    echo -e "${YELLOW}⚠️  No tags found. Starting with v0.1.0${NC}"
    new_tag="v0.1.0"
else
    echo -e "${GREEN}✅ Latest tag: $latest_tag${NC}"
    
    # Extract version number and bump it
    if [[ $latest_tag =~ ^v([0-9]+)\.([0-9]+)\.([0-9]+)$ ]]; then
        major=${BASH_REMATCH[1]}
        minor=${BASH_REMATCH[2]}
        patch=${BASH_REMATCH[3]}
        
        echo "Current version: $major.$minor.$patch"
        echo "What would you like to bump?"
        echo "1) Patch (v$major.$minor.$((patch + 1)))"
        echo "2) Minor (v$major.$((minor + 1)).0)"
        echo "3) Major (v$((major + 1)).0.0)"
        echo "4) Custom"
        
        read -p "Choose (1-4): " -n 1 -r
        echo
        
        case $REPLY in
            1)
                new_tag="v$major.$minor.$((patch + 1))"
                ;;
            2)
                new_tag="v$major.$((minor + 1)).0"
                ;;
            3)
                new_tag="v$((major + 1)).0.0"
                ;;
            4)
                read -p "Enter custom tag (e.g., v1.2.3): " new_tag
                ;;
            *)
                echo -e "${RED}❌ Invalid choice. Exiting.${NC}"
                exit 1
                ;;
        esac
    else
        echo -e "${YELLOW}⚠️  Latest tag doesn't follow semantic versioning: $latest_tag${NC}"
        read -p "Enter new tag (e.g., v1.0.0): " new_tag
    fi
fi

echo -e "${BLUE}📝 New tag will be: $new_tag${NC}"

# Ask for confirmation
read -p "Create and push tag '$new_tag'? (y/N): " -n 1 -r
echo

if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo "Creating annotated tag..."
    git tag -a "$new_tag" -m "Release $new_tag"
    
    echo "Pushing tag to origin..."
    git push origin "$new_tag"
    
    echo -e "${GREEN}✅ Successfully created and pushed tag: $new_tag${NC}"
else
    echo -e "${YELLOW}⚠️  Tag creation cancelled.${NC}"
    echo "To create the tag manually, run:"
    echo "  git tag -a $new_tag -m \"Release $new_tag\""
    echo "  git push origin $new_tag"
fi
