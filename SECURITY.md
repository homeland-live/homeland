# Security

## Reporting a vulnerability

Please report vulnerabilities privately through GitHub's private vulnerability reporting (the repository's
**Security** tab, then **Report a vulnerability**), not in a public issue. Include the version, how to reproduce,
and the impact you see.

Fixes go into the latest release only, so please check that the issue still exists there.

## Security model

Homeland is built for a trusted home network. Keep these limits in mind when deploying it and when judging whether
something is a vulnerability:

- **The HTTP API and UI have no authentication.** They expose the device list, device state, event history and the
  source of every script. Don't publish the port to the internet; put an authenticating reverse proxy in front if
  you need remote access.
- **Scripts are trusted code.** Each runs in its own goja runtime with no file, network or process access beyond
  `ctx`, and `HL_SCRIPTS_TIMEOUT` bounds every run, but goja is not a security sandbox. Anyone who can write to
  `HL_SCRIPTS_DIR` can control your devices.
- **The MQTT brokers are trusted.** Anyone who can publish to the Zigbee2MQTT base topic can trigger scripts.
  Protect the broker with credentials and ACLs.
- **Credentials come from the environment** (`HL_*_MQTT_PASSWORD`). Keep `.env` files and compose files with real
  secrets out of version control.

Reports that bypass these assumptions are still welcome, for example a script escaping its runtime, a request that
crashes the service, or a path that reads files outside `HL_SCRIPTS_DIR`.
