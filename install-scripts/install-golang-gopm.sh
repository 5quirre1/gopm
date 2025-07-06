#!/bin/bash
# made by nameswastaken!1!
sudo apt update
sudo apt install -y git curl build-essential
if ! command -v go >/dev/null 2>&1; then
    curl -LO https://go.dev/dl/go1.24.4.linux-amd64.tar.gz
    sudo tar -C /usr/local -xzf go*.linux-amd64.tar.gz
fi
export PATH=$PATH:/usr/local/go/bin
git clone https://github.com/5quirre1/gopm.git
cd gopm
go build -o gopm
CURRENT_DIR="$(pwd)"
SHELL_NAME=$(basename "$SHELL")

case "$SHELL_NAME" in
    bash)
        CONFIG_FILE="$HOME/.bashrc"
        [[ -f "$HOME/.bash_profile" ]] && CONFIG_FILE="$HOME/.bash_profile"
        ;;
    zsh)
        CONFIG_FILE="$HOME/.zshrc"
        ;;
    fish)
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
echo ""
echo "testing gopm installation... (thank squirrel for part of script)"
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
source ~/.bashrc
