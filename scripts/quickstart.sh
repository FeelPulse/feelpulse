#!/bin/bash
# FeelPulse Quickstart Script
# ============================
# Gets you from zero to running in one script.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/FeelPulse/feelpulse/main/scripts/quickstart.sh | bash
#   OR
#   ./scripts/quickstart.sh

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Banner
echo ""
echo -e "${CYAN}╔═══════════════════════════════════════════╗${NC}"
echo -e "${CYAN}║${NC}          🫀 ${BLUE}FeelPulse Quickstart${NC}           ${CYAN}║${NC}"
echo -e "${CYAN}║${NC}     Fast, minimal AI assistant in Go      ${CYAN}║${NC}"
echo -e "${CYAN}╚═══════════════════════════════════════════╝${NC}"
echo ""

# Helper functions
info() { echo -e "${BLUE}ℹ${NC}  $1"; }
success() { echo -e "${GREEN}✓${NC}  $1"; }
warning() { echo -e "${YELLOW}⚠${NC}  $1"; }
error() { echo -e "${RED}✗${NC}  $1"; exit 1; }
prompt() { echo -en "${CYAN}?${NC}  $1"; }

# Check if command exists
check_command() {
    if command -v "$1" &> /dev/null; then
        return 0
    else
        return 1
    fi
}

# =============================================================================
# Step 1: Check Prerequisites
# =============================================================================
echo -e "${YELLOW}Step 1/5: Checking prerequisites...${NC}"

# Check Go
if check_command go; then
    GO_VERSION=$(go version | awk '{print $3}')
    success "Go installed: $GO_VERSION"
else
    error "Go is not installed. Please install Go 1.23+ from https://go.dev/dl/"
fi

# Check git
if check_command git; then
    success "Git installed"
else
    error "Git is not installed. Please install git."
fi

echo ""

# =============================================================================
# Step 2: Clone and Build
# =============================================================================
echo -e "${YELLOW}Step 2/5: Building FeelPulse...${NC}"

# Check if we're already in the feelpulse directory
if [ -f "go.mod" ] && grep -q "feelpulse" go.mod 2>/dev/null; then
    info "Already in FeelPulse directory"
    FEELPULSE_DIR="$(pwd)"
else
    # Clone if not exists
    if [ -d "feelpulse" ]; then
        info "Directory exists, pulling latest..."
        cd feelpulse
        git pull
        FEELPULSE_DIR="$(pwd)"
    else
        info "Cloning repository..."
        git clone https://github.com/FeelPulse/feelpulse.git
        cd feelpulse
        FEELPULSE_DIR="$(pwd)"
    fi
fi

# Build
info "Building binary..."
go build -o build/feelpulse ./cmd/feelpulse
success "Built: $FEELPULSE_DIR/build/feelpulse"

echo ""

# =============================================================================
# Step 3: Initialize Configuration
# =============================================================================
echo -e "${YELLOW}Step 3/5: Initializing configuration...${NC}"

CONFIG_DIR="$HOME/.feelpulse"
CONFIG_FILE="$CONFIG_DIR/config.yaml"

if [ -f "$CONFIG_FILE" ]; then
    warning "Config already exists at $CONFIG_FILE"
    prompt "Overwrite? (y/N): "
    read -r OVERWRITE
    if [ "$OVERWRITE" != "y" ] && [ "$OVERWRITE" != "Y" ]; then
        info "Keeping existing config"
    else
        ./build/feelpulse init
        success "Config initialized"
    fi
else
    ./build/feelpulse init
    success "Config created at $CONFIG_FILE"
fi

# Initialize workspace
if [ ! -d "$CONFIG_DIR/workspace" ] || [ ! -f "$CONFIG_DIR/workspace/SOUL.md" ]; then
    ./build/feelpulse workspace init
    success "Workspace initialized"
else
    info "Workspace already exists"
fi

echo ""

# =============================================================================
# Step 4: Configure Authentication
# =============================================================================
echo -e "${YELLOW}Step 4/5: Configuring authentication...${NC}"

echo ""
echo "How do you want to authenticate?"
echo "  1) Anthropic API key (from console.anthropic.com)"
echo "  2) Claude subscription token (from 'claude setup-token')"
echo "  3) Skip (configure manually later)"
echo ""

prompt "Choice [1/2/3]: "
read -r AUTH_CHOICE

case "$AUTH_CHOICE" in
    1)
        prompt "Enter your Anthropic API key (sk-ant-api...): "
        read -rs API_KEY
        echo ""
        if [ -n "$API_KEY" ]; then
            # Update config with sed (cross-platform)
            if [[ "$OSTYPE" == "darwin"* ]]; then
                sed -i '' "s/apiKey: \"\"/apiKey: \"$API_KEY\"/" "$CONFIG_FILE"
            else
                sed -i "s/apiKey: \"\"/apiKey: \"$API_KEY\"/" "$CONFIG_FILE"
            fi
            success "API key configured"
        else
            warning "No key entered, skipping"
        fi
        ;;
    2)
        prompt "Enter your Claude setup token (sk-ant-oat...): "
        read -rs AUTH_TOKEN
        echo ""
        if [ -n "$AUTH_TOKEN" ]; then
            if [[ "$OSTYPE" == "darwin"* ]]; then
                sed -i '' "s/authToken: \"\"/authToken: \"$AUTH_TOKEN\"/" "$CONFIG_FILE"
            else
                sed -i "s/authToken: \"\"/authToken: \"$AUTH_TOKEN\"/" "$CONFIG_FILE"
            fi
            success "Auth token configured"
        else
            warning "No token entered, skipping"
        fi
        ;;
    *)
        info "Skipping auth configuration"
        info "Run 'feelpulse auth' later to configure"
        ;;
esac

echo ""

# =============================================================================
# Step 5: Configure Telegram
# =============================================================================
echo -e "${YELLOW}Step 5/5: Configuring Telegram...${NC}"

echo ""
prompt "Do you have a Telegram bot token? (y/N): "
read -r HAS_TOKEN

if [ "$HAS_TOKEN" = "y" ] || [ "$HAS_TOKEN" = "Y" ]; then
    prompt "Enter your Telegram bot token: "
    read -r TG_TOKEN
    echo ""
    
    if [ -n "$TG_TOKEN" ]; then
        # Enable telegram and set token
        if [[ "$OSTYPE" == "darwin"* ]]; then
            sed -i '' 's/enabled: false/enabled: true/' "$CONFIG_FILE"
            sed -i '' "s/token: \"\"/token: \"$TG_TOKEN\"/" "$CONFIG_FILE"
        else
            sed -i 's/enabled: false/enabled: true/' "$CONFIG_FILE"
            sed -i "s/token: \"\"/token: \"$TG_TOKEN\"/" "$CONFIG_FILE"
        fi
        success "Telegram configured"
        
        prompt "Enter your Telegram username (for allowlist, optional): "
        read -r TG_USER
        if [ -n "$TG_USER" ]; then
            # Add to allowedUsers (basic approach)
            if [[ "$OSTYPE" == "darwin"* ]]; then
                sed -i '' "s/allowedUsers: \[\]/allowedUsers: [\"$TG_USER\"]/" "$CONFIG_FILE"
            else
                sed -i "s/allowedUsers: \[\]/allowedUsers: [\"$TG_USER\"]/" "$CONFIG_FILE"
            fi
            success "Added $TG_USER to allowed users"
        else
            warning "No allowlist configured - bot will be public!"
        fi
    else
        warning "No token entered, skipping Telegram setup"
    fi
else
    info "Skipping Telegram setup"
    info "Create a bot: https://t.me/BotFather"
    info "Then add the token to $CONFIG_FILE"
fi

echo ""

# =============================================================================
# Done!
# =============================================================================
echo -e "${GREEN}╔═══════════════════════════════════════════╗${NC}"
echo -e "${GREEN}║${NC}        🎉 ${BLUE}Setup Complete!${NC}                  ${GREEN}║${NC}"
echo -e "${GREEN}╚═══════════════════════════════════════════╝${NC}"
echo ""

echo "Configuration: $CONFIG_FILE"
echo "Workspace:     $CONFIG_DIR/workspace/"
echo "Binary:        $FEELPULSE_DIR/build/feelpulse"
echo ""

echo "📝 Next steps:"
echo ""
echo "  1. Review your config:"
echo "     ${CYAN}cat ~/.feelpulse/config.yaml${NC}"
echo ""
echo "  2. Start FeelPulse:"
echo "     ${CYAN}$FEELPULSE_DIR/build/feelpulse start${NC}"
echo ""
echo "  3. Or install as service:"
echo "     ${CYAN}$FEELPULSE_DIR/build/feelpulse service install${NC}"
echo "     ${CYAN}$FEELPULSE_DIR/build/feelpulse service enable${NC}"
echo ""
echo "  4. Try the TUI chat:"
echo "     ${CYAN}$FEELPULSE_DIR/build/feelpulse tui${NC}"
echo ""

echo "📚 Documentation: https://github.com/FeelPulse/feelpulse"
echo ""
echo "Happy pulsing! 🫀"
