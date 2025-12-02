#!/bin/bash
# Post-installation script for NFC Agent

# Ensure pcscd is running (required for NFC readers)
if command -v systemctl &> /dev/null; then
    # Enable and start pcscd if available
    systemctl enable pcscd 2>/dev/null || true
    systemctl start pcscd 2>/dev/null || true
fi

# Copy systemd user service to system location for all users
if [ -d /usr/lib/systemd/user ]; then
    echo "Systemd user service installed to /usr/lib/systemd/user/nfc-agent.service"
    echo ""
    echo "To enable auto-start for your user, run:"
    echo "  systemctl --user daemon-reload"
    echo "  systemctl --user enable --now nfc-agent"
fi

echo ""
echo "NFC Agent installed successfully!"
echo "Run 'nfc-agent' to start the service manually."
echo "Visit http://127.0.0.1:32145 to access the status page."
