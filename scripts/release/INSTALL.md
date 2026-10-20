# Install Unity Gateway on macOS

> **Beta:** Unity Gateway is currently a public beta. The CLI and app are
> ad-hoc signed but are not Apple-notarized. macOS may require explicit
> approval before the app opens for the first time.

Homebrew is the canonical public install and upgrade channel. This one
command installs the Databricks CLI and Unity Gateway from Databricks's public tap:

```sh
brew install databrickslabs/databricks/unity-gateway
```

Then authenticate and start the gateway:

```sh
unity-gateway setup
unity-gateway up --install
unity-gateway claude on
unity-gateway doctor --probe
```

## Direct release asset

Approved direct installs use
`unity-gateway_<version>_darwin_universal.zip`. The release also publishes
`checksums.txt`. Verify the ZIP's SHA-256 entry before extracting it.

The release ZIP contains the universal, ad-hoc signed CLI, a nested ad-hoc signed
`Unity Gateway.app.zip`, and `install.sh`. Run:

```sh
unzip unity-gateway_<version>_darwin_universal.zip \
  -d unity-gateway_<version>
cd unity-gateway_<version>
./install.sh
```

The installer validates the ad-hoc signatures, matching versions, and universal
architectures. An ad-hoc signature detects changes after packaging, but it does
not establish an Apple-verified developer identity or notarization.

## First launch approval

If macOS blocks the beta app on first launch:

1. In Finder, open `~/Applications`, double-click **Unity Gateway**, then
   dismiss the warning.
2. Choose **Apple menu > System Settings > Privacy & Security**.
3. Scroll to the **Security** section and click **Open Anyway** for Databricks
   Switch.
4. Confirm **Open** and authenticate if macOS asks.

The **Open Anyway** button is available for about one hour after the blocked
launch attempt. A managed Mac may prevent this override. Unity Gateway does
not ask users to clear quarantine metadata or disable Gatekeeper.

## Uninstall

```sh
unity-gateway uninstall --dry-run
unity-gateway uninstall
```

Use `unity-gateway uninstall --purge --yes` to remove retained Unity Gateway
config, telemetry, logs, and backups. Databricks CLI credentials and keychain
entries are never removed. The command prints manual instructions when the
macOS app's Start at Login item prevents safe automated bundle removal.

The repository README contains the supported install, operation,
troubleshooting, and uninstall instructions.
