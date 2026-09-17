#!/usr/bin/env node
'use strict';

const crypto = require('node:crypto');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { spawnSync } = require('node:child_process');

const OWNER = 'acumen-ai-org';
const REPO = 'robotdreams';

const packageDir = path.resolve(__dirname, '..');
const pkg = require(path.join(packageDir, 'package.json'));

function resolveTarget(version, platform, arch) {
  const goos = { linux: 'linux', darwin: 'darwin', win32: 'windows' }[platform];
  const goarch = { x64: 'amd64', arm64: 'arm64' }[arch];
  if (!goos || !goarch) {
    throw new Error(
      `robotdreams: no prebuilt dream binary for ${platform}/${arch}.\n` +
        'Supported platforms: linux, macOS, windows on x64/arm64.\n' +
        `Alternative (needs a Go toolchain): go install github.com/${OWNER}/${REPO}/cmd/dream@latest`
    );
  }
  const ext = goos === 'windows' ? 'zip' : 'tar.gz';
  return {
    goos,
    goarch,
    archiveName: `dream_${version}_${goos}_${goarch}.${ext}`,
    binaryName: goos === 'windows' ? 'dream.exe' : 'dream',
  };
}

function binaryPath(platform) {
  const p = platform || process.platform;
  return path.join(packageDir, 'dist', p === 'win32' ? 'dream.exe' : 'dream');
}

function authToken() {
  return process.env.GITHUB_TOKEN || process.env.GH_TOKEN || '';
}

async function fetchOk(url, headers) {
  const res = await fetch(url, { headers, redirect: 'follow' });
  if (!res.ok) {
    throw new Error(`GET ${url} failed: HTTP ${res.status} ${res.statusText}`);
  }
  return res;
}

async function downloadAsset(version, assetName) {
  const token = authToken();
  const ua = { 'user-agent': `robotdreams-npm/${pkg.version}` };
  if (!token) {
    const url = `https://github.com/${OWNER}/${REPO}/releases/download/v${version}/${assetName}`;
    const res = await fetchOk(url, ua);
    return Buffer.from(await res.arrayBuffer());
  }
  const apiHeaders = {
    ...ua,
    authorization: `Bearer ${token}`,
    accept: 'application/vnd.github+json',
    'x-github-api-version': '2022-11-28',
  };
  const relUrl = `https://api.github.com/repos/${OWNER}/${REPO}/releases/tags/v${version}`;
  const release = await (await fetchOk(relUrl, apiHeaders)).json();
  const asset = (release.assets || []).find((a) => a.name === assetName);
  if (!asset) {
    throw new Error(`release v${version} has no asset named ${assetName}`);
  }
  const res = await fetchOk(`https://api.github.com/repos/${OWNER}/${REPO}/releases/assets/${asset.id}`, {
    ...apiHeaders,
    accept: 'application/octet-stream',
  });
  return Buffer.from(await res.arrayBuffer());
}

function checksumFor(checksumsText, archiveName) {
  for (const line of checksumsText.split('\n')) {
    const m = line.trim().match(/^([0-9a-fA-F]{64})[ *]+(.+)$/);
    if (m && m[2] === archiveName) return m[1].toLowerCase();
  }
  throw new Error(`checksums.txt has no entry for ${archiveName}`);
}

function extractBinary(archiveBuf, archiveName, binaryName) {
  const tmp = fs.mkdtempSync(path.join(os.tmpdir(), 'robotdreams-'));
  try {
    const archivePath = path.join(tmp, archiveName);
    fs.writeFileSync(archivePath, archiveBuf);
    const r = spawnSync('tar', ['-xf', archivePath, '-C', tmp], { stdio: ['ignore', 'inherit', 'inherit'] });
    if (r.error) throw r.error;
    if (r.status !== 0) throw new Error(`tar exited with status ${r.status}`);
    const extracted = path.join(tmp, binaryName);
    if (!fs.existsSync(extracted)) {
      throw new Error(`archive ${archiveName} did not contain ${binaryName}`);
    }
    const dest = path.join(packageDir, 'dist', binaryName);
    fs.mkdirSync(path.dirname(dest), { recursive: true });
    fs.copyFileSync(extracted, dest);
    fs.chmodSync(dest, 0o755);
    return dest;
  } finally {
    fs.rmSync(tmp, { recursive: true, force: true });
  }
}

async function install() {
  if (process.env.ROBOTDREAMS_SKIP_DOWNLOAD) {
    console.log('robotdreams: ROBOTDREAMS_SKIP_DOWNLOAD set, skipping binary download.');
    return null;
  }
  const version = pkg.version;
  const target = resolveTarget(version, process.platform, process.arch);
  try {
    const [archive, checksums] = await Promise.all([
      downloadAsset(version, target.archiveName),
      downloadAsset(version, 'checksums.txt'),
    ]);
    const want = checksumFor(checksums.toString('utf8'), target.archiveName);
    const got = crypto.createHash('sha256').update(archive).digest('hex');
    if (got !== want) {
      throw new Error(`SHA-256 mismatch for ${target.archiveName}: expected ${want}, got ${got}`);
    }
    const dest = extractBinary(archive, target.archiveName, target.binaryName);
    console.log(`robotdreams: installed dream ${version} (${target.goos}/${target.goarch}) at ${dest}`);
    return dest;
  } catch (err) {
    throw new Error(
      `robotdreams: could not fetch the prebuilt dream binary for v${version} ` +
        `(${target.goos}/${target.goarch}).\n` +
        `  Cause: ${err.message}\n` +
        'Likely reasons:\n' +
        `  - the GitHub repository (github.com/${OWNER}/${REPO}) is private: set GITHUB_TOKEN\n` +
        '    (or GH_TOKEN) to a token that can read its releases, then reinstall;\n' +
        `  - release tag v${version} does not exist yet, or is missing the asset\n` +
        `    ${target.archiveName}.\n` +
        'Alternative (needs a Go toolchain):\n' +
        `  go install github.com/${OWNER}/${REPO}/cmd/dream@latest`
    );
  }
}

function selfTest() {
  const assert = require('node:assert');
  const v = '1.2.3';
  const cases = [
    ['linux', 'x64', 'linux', 'amd64', `dream_${v}_linux_amd64.tar.gz`, 'dream'],
    ['linux', 'arm64', 'linux', 'arm64', `dream_${v}_linux_arm64.tar.gz`, 'dream'],
    ['darwin', 'x64', 'darwin', 'amd64', `dream_${v}_darwin_amd64.tar.gz`, 'dream'],
    ['darwin', 'arm64', 'darwin', 'arm64', `dream_${v}_darwin_arm64.tar.gz`, 'dream'],
    ['win32', 'x64', 'windows', 'amd64', `dream_${v}_windows_amd64.zip`, 'dream.exe'],
    ['win32', 'arm64', 'windows', 'arm64', `dream_${v}_windows_arm64.zip`, 'dream.exe'],
  ];
  for (const [platform, arch, goos, goarch, archiveName, binaryName] of cases) {
    const t = resolveTarget(v, platform, arch);
    assert.deepStrictEqual(
      { goos: t.goos, goarch: t.goarch, archiveName: t.archiveName, binaryName: t.binaryName },
      { goos, goarch, archiveName, binaryName },
      `mapping for ${platform}/${arch}`
    );
  }
  for (const [platform, arch] of [['freebsd', 'x64'], ['linux', 'ia32'], ['sunos', 'mips']]) {
    assert.throws(() => resolveTarget(v, platform, arch), /no prebuilt dream binary/, `${platform}/${arch} must throw`);
  }
  assert.strictEqual(
    checksumFor('deadbeef\nabc\n' + 'a'.repeat(64) + `  dream_${v}_linux_amd64.tar.gz\n`, `dream_${v}_linux_amd64.tar.gz`),
    'a'.repeat(64)
  );
  assert.throws(() => checksumFor('', 'nope.tar.gz'), /no entry/);
  console.log(`robotdreams self-test: ${cases.length} platform mappings + error cases OK`);
}

module.exports = { resolveTarget, binaryPath, checksumFor, install };

if (require.main === module) {
  if (process.argv.includes('--self-test')) {
    selfTest();
  } else {
    install().catch((err) => {
      console.error(err.message);
      process.exit(1);
    });
  }
}
