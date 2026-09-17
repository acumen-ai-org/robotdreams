#!/usr/bin/env node
// Launcher for the `dream` CLI. The real binary is a prebuilt Go
// executable downloaded by scripts/install.js (normally at npm install
// time via postinstall). This shim just spawns it, passing through
// arguments, stdio, and the exit code. If the binary is missing (for
// example the package was installed with --ignore-scripts), it downloads
// it first.
'use strict';

const fs = require('node:fs');
const { spawnSync } = require('node:child_process');
const installer = require('../scripts/install.js');

async function main() {
  const bin = installer.binaryPath();
  if (!fs.existsSync(bin)) {
    await installer.install();
    if (!fs.existsSync(bin)) {
      throw new Error('robotdreams: dream binary unavailable after install attempt.');
    }
  }
  // Tell the binary how it was installed, so `dream self-update` refuses
  // to overwrite it and points at npm instead. Detection also works from
  // the executable's path alone, but this makes it exact rather than
  // shape-based — and it keeps working if the package layout ever moves.
  // Keep the value in sync with selfupdate.MethodNPM.
  const env = { ...process.env, ROBOTDREAMS_INSTALL_METHOD: 'npm' };
  const result = spawnSync(bin, process.argv.slice(2), { stdio: 'inherit', env });
  if (result.error) throw result.error;
  if (result.signal) {
    // Child died from a signal: re-raise it so our caller sees the same.
    process.kill(process.pid, result.signal);
    return;
  }
  process.exit(result.status === null ? 1 : result.status);
}

main().catch((err) => {
  console.error(err.message || err);
  process.exit(1);
});
