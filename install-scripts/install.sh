#!/bin/bash
if [[ ! -f "gopm" ]]; then
    echo "error: gopm not found in current directory"
    echo "please run this script from the directory containing gopm"
    exit 1
fi
chmod +x gopm
CURRENT_DIR="$(pwd)"
if echo "$PATH" | grep -q "$CURRENT_DIR"; then
    echo "gopm is already in PATH"
else
    echo "adding gopm to PATH..."
    SHELL_NAME=$(basename "$SHELL")
    case "$SHELL_NAME" in
        "bash")
            if [[ -f "$HOME/.bash_profile" ]]; then
                CONFIG_FILE="$HOME/.bash_profile"
            else
                CONFIG_FILE="$HOME/.bashrc"
            fi
            ;;
        "zsh")
            CONFIG_FILE="$HOME/.zshrc"
            ;;
        "fish")
            CONFIG_FILE="$HOME/.config/fish/config.fish"
            ;;
        *)
            CONFIG_FILE="$HOME/.profile"
            ;;
    esac
    if [[ "$SHELL_NAME" == "fish" ]]; then
        echo "set -gx PATH \$PATH $CURRENT_DIR" >> "$CONFIG_FILE"
    else
        echo "export PATH=\"\$PATH:$CURRENT_DIR\"" >> "$CONFIG_FILE"
    fi
    export PATH="$PATH:$CURRENT_DIR"
    echo "successfully added gopm to PATH"
    echo ""
    echo "important: you need to restart your command prompt or terminal"
    echo "for the changes to take effect, or run:"
    echo "  source $CONFIG_FILE"
fi
echo ""
echo "testing gopm installation..."
if command -v gopm >/dev/null 2>&1; then
    gopm version
    echo ""
    echo "gopm is working correctly!!"
    echo "you can now use gopm !!!"
else
    echo ""
    echo "gopm test failed.. you may need to restart your terminal.."
fi
echo ""
echo "installation complete!!!"
