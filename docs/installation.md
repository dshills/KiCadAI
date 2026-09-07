# Installing KiCadAI v1.0.1

Use the assets from the [v1.0.1 release](https://github.com/dshills/KiCadAI/releases/tag/v1.0.1).
Official binaries need no Go installation. Choose exactly one platform:

| Operating system | Architecture | Asset |
| --- | --- | --- |
| macOS | Intel | `kicadai_v1.0.1_darwin_amd64` |
| macOS | Apple silicon | `kicadai_v1.0.1_darwin_arm64` |
| Linux | x86-64 | `kicadai_v1.0.1_linux_amd64` |
| Linux | ARM64 | `kicadai_v1.0.1_linux_arm64` |

Also download `RELEASE_MANIFEST.json` and `SHA256SUMS` from that same release.
Do not execute the binary before verifying it. In the download directory, set
`asset` to your exact filename and verify both downloaded files:

```sh
asset=kicadai_v1.0.1_darwin_arm64 # Change for your platform.
awk -v asset="$asset" '$2 == asset { print; binary++ }
  $2 == "RELEASE_MANIFEST.json" { print; manifest++ }
  END { if (binary != 1 || manifest != 1) exit 1 }' SHA256SUMS > selected-checksums.txt &&
  shasum -a 256 -c selected-checksums.txt
```

On Linux, `sha256sum -c selected-checksums.txt` may be used instead of `shasum`.
Stop on any missing entry, checksum failure, or unexpected filename. Checksums
detect corruption and permit comparison with a source build; they are not a
signature or an independent trust root. Obtain them from the official release
over HTTPS. The manifest records the source commit, toolchain, and build date.

Only after both checks pass:

```sh
mkdir -p "$HOME/.local/bin"
install -m 0755 "$asset" "$HOME/.local/bin/kicadai"
export PATH="$HOME/.local/bin:$PATH"
"$HOME/.local/bin/kicadai" --help
"$HOME/.local/bin/kicadai" version
"$HOME/.local/bin/kicadai" capability generation
"$HOME/.local/bin/kicadai" --format json --document first-run plan-led-demo
```

The version command must report `1.0.1`. These first checks use the installed
path explicitly so an existing alias or shell command cache cannot select an
older binary. Persist the PATH change in your shell configuration if needed.
The LED command
produces a supported transaction plan without a KiCad connection; it is not
installed-KiCad validation. KiCad 10.0.3 is required for the documented reference
ERC/strict-DRC/promotion claims. KiCad 9 remains experimental for those claims.
Windows is not an official release target. On macOS, follow your organization's
normal policy for unsigned downloaded command-line tools; do not disable
system-wide protections.

## Building from the release source

Source builds require Go 1.26.8 or newer, Git, and Make. Canonical Make targets
select Go 1.26.8 from `go.mod` and may download that toolchain. `protoc` is needed
only for protobuf regeneration.

```sh
git clone --branch v1.0.1 https://github.com/dshills/KiCadAI.git
cd KiCadAI
make install
```

`make install` compiles a development-identity binary into `~/.local/bin` by
default; use `INSTALL_DIR` to override the destination. It does not impersonate
an official release build. For versioned, reproducible release identity, use
`make release` from a clean tagged checkout, then verify and install the
matching `dist/` binary as above. Do not use `ALLOW_DIRTY_RELEASE=1` for published
or authenticated artifacts. `go install ...@version` is not the CLI installation
contract.

For an installed-KiCad public example, follow the
[protected-current demo](../examples/public-demo/protected-programmable-current-output/README.md).
All generated candidates retain the [v1 support and refusal boundary](../SUPPORT.md).
This patch does not admit V21; its corrected evaluator awaits a separate frozen
public evaluation.
