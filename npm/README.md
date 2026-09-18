# robotdreams (npm wrapper for the `dream` CLI)

Robot Dreams is a lightweight control plane for fleets of autonomous
software agents. This npm package installs the prebuilt `dream` CLI
binary for your platform — no Go toolchain required.

```sh
npm install -g robotdreams   # installs the `dream` command
dream --help

# or one-off, without installing:
npx robotdreams -- --help
```

Supported platforms: Linux, macOS, and Windows on x64/arm64. On install,
a postinstall script downloads the matching binary from the GitHub
Release for this package's version and verifies its SHA-256 checksum. If
the repository is private (or you hit API rate limits), set
`GITHUB_TOKEN` to a token that can read its releases.

Project home, source, and full documentation:
https://github.com/acumen-ai-org/robotdreams
