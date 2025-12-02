#!/usr/bin/env node

const https = require('https');
const fs = require('fs');
const path = require('path');
const { execSync } = require('child_process');

const pkg = require('../package.json');
const version = pkg.version;

const PLATFORM_MAP = {
  darwin: 'macos',
  linux: 'linux',
  win32: 'windows'
};

const ARCH_MAP = {
  x64: 'amd64',
  arm64: 'arm64'
};

function getPlatformBinary() {
  const platform = PLATFORM_MAP[process.platform];
  const arch = ARCH_MAP[process.arch];

  if (!platform) {
    throw new Error(`Unsupported platform: ${process.platform}`);
  }
  if (!arch) {
    throw new Error(`Unsupported architecture: ${process.arch}`);
  }

  // Map to release asset names
  const ext = process.platform === 'win32' ? '.exe' : '';

  if (platform === 'macos') {
    // macOS uses universal binary in DMG, but we can use the standalone from release
    return `NFC-Agent-${version}-macos.dmg`;
  } else if (platform === 'windows') {
    return `NFC-Agent-${version}-windows${ext}`;
  } else {
    // Linux - use deb/rpm or build from source
    return `NFC-Agent-${version}-linux-${arch}.deb`;
  }
}

function downloadBinary(url, dest) {
  return new Promise((resolve, reject) => {
    const file = fs.createWriteStream(dest);

    https.get(url, {
      headers: {
        'User-Agent': 'nfc-agent-npm-installer',
        'Accept': 'application/octet-stream'
      }
    }, (response) => {
      // Handle redirects
      if (response.statusCode === 302 || response.statusCode === 301) {
        file.close();
        fs.unlinkSync(dest);
        return downloadBinary(response.headers.location, dest).then(resolve).catch(reject);
      }

      if (response.statusCode !== 200) {
        file.close();
        fs.unlinkSync(dest);
        reject(new Error(`Failed to download: ${response.statusCode}`));
        return;
      }

      response.pipe(file);
      file.on('finish', () => {
        file.close();
        resolve();
      });
    }).on('error', (err) => {
      file.close();
      fs.unlinkSync(dest);
      reject(err);
    });
  });
}

async function install() {
  const binDir = path.join(__dirname, '..', 'bin');
  const binName = process.platform === 'win32' ? 'nfc-agent.exe' : 'nfc-agent';
  const binPath = path.join(binDir, binName);

  // For macOS/Windows, download the standalone exe directly
  // For Linux, the user should use the .deb/.rpm package instead

  if (process.platform === 'linux') {
    console.log('NFC Agent: On Linux, please install via .deb or .rpm package:');
    console.log(`  Download from: https://github.com/SimplyPrint/nfc-agent/releases/tag/v${version}`);
    console.log('  Or use: sudo dpkg -i NFC-Agent-*.deb');

    // Create a stub script that shows the message
    const stubScript = `#!/bin/sh
echo "NFC Agent is not installed via npm on Linux."
echo "Please install the .deb or .rpm package from:"
echo "  https://github.com/SimplyPrint/nfc-agent/releases"
exit 1
`;
    fs.writeFileSync(binPath, stubScript);
    fs.chmodSync(binPath, 0o755);
    return;
  }

  const assetName = process.platform === 'win32'
    ? `NFC-Agent-${version}-windows.exe`
    : null; // macOS needs special handling due to DMG

  if (process.platform === 'darwin') {
    console.log('NFC Agent: On macOS, please install via DMG for best experience:');
    console.log(`  Download from: https://github.com/SimplyPrint/nfc-agent/releases/tag/v${version}`);

    // Create a stub script
    const stubScript = `#!/bin/sh
echo "NFC Agent is not installed via npm on macOS."
echo "Please install the .dmg package from:"
echo "  https://github.com/SimplyPrint/nfc-agent/releases"
exit 1
`;
    fs.writeFileSync(binPath, stubScript);
    fs.chmodSync(binPath, 0o755);
    return;
  }

  // Windows - download the exe directly
  const url = `https://github.com/SimplyPrint/nfc-agent/releases/download/v${version}/${assetName}`;

  console.log(`Downloading NFC Agent v${version} for ${process.platform}...`);

  try {
    await downloadBinary(url, binPath);

    if (process.platform !== 'win32') {
      fs.chmodSync(binPath, 0o755);
    }

    console.log('NFC Agent installed successfully!');
  } catch (err) {
    console.error('Failed to download NFC Agent:', err.message);
    console.log(`Please download manually from: https://github.com/SimplyPrint/nfc-agent/releases/tag/v${version}`);
    process.exit(1);
  }
}

install().catch(err => {
  console.error(err);
  process.exit(1);
});
