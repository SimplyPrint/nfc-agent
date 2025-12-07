#!/bin/bash
# Post-installation script for NFC Agent

# Unload kernel NFC modules that conflict with pcscd (if loaded)
# The blacklist config file prevents them from loading on next boot
if lsmod | grep -q pn533; then
    echo "Unloading conflicting kernel NFC modules..."
    modprobe -r pn533_usb 2>/dev/null || true
    modprobe -r pn533 2>/dev/null || true
    modprobe -r nfc 2>/dev/null || true
fi

# Ensure pcscd is running (required for NFC readers)
if command -v systemctl &> /dev/null; then
    # Enable and start pcscd if available
    systemctl enable pcscd 2>/dev/null || true
    systemctl restart pcscd 2>/dev/null || true
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
