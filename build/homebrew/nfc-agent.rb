# Homebrew formula for NFC Agent
# This file should be placed in SimplyPrint/homebrew-tap repository

class NfcAgent < Formula
  desc "Local NFC card reader service for web applications"
  homepage "https://github.com/SimplyPrint/nfc-agent"
  version "1.0.0"
  license "MIT"

  on_macos do
    on_arm do
      url "https://github.com/SimplyPrint/nfc-agent/releases/download/v#{version}/nfc-agent_#{version}_darwin_arm64.tar.gz"
      # sha256 will be auto-filled by homebrew-bump-formula action
    end
    on_intel do
      url "https://github.com/SimplyPrint/nfc-agent/releases/download/v#{version}/nfc-agent_#{version}_darwin_amd64.tar.gz"
      # sha256 will be auto-filled by homebrew-bump-formula action
    end
  end

  on_linux do
    on_intel do
      url "https://github.com/SimplyPrint/nfc-agent/releases/download/v#{version}/nfc-agent_#{version}_linux_amd64.tar.gz"
      # sha256 will be auto-filled by homebrew-bump-formula action
    end
  end

  def install
    bin.install "nfc-agent"
  end

  service do
    run [opt_bin/"nfc-agent", "--no-tray"]
    keep_alive true
    log_path var/"log/nfc-agent.log"
    error_log_path var/"log/nfc-agent.err"
  end

  test do
    assert_match "nfc-agent", shell_output("#{bin}/nfc-agent version")
  end
end
