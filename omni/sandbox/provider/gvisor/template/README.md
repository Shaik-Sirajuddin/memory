# gVisor OCI Template

- `config.json`: expected OCI runtime config format for gVisor bundle creation.
- `oci-bundle-layout.txt`: expected OCI bundle directory listing.
- `link-host-binaries.sh`: creates host-path symlinks under `rootfs` for common bins/libs.

Use this folder as the reference when creating or validating `ProvisionerOptions.WorkDir` for gVisor.

Example:

```sh
sh sandbox/provider/gvisor/template/link-host-binaries.sh
```

Notes:

- With the current `config.json` bind mounts (`/bin`, `/sbin`, `/lib*`, `/usr/*`, `/etc`), you typically do **not** need to run `link-host-binaries.sh`.
- Use `link-host-binaries.sh` only if you want a rootfs that also mirrors those host paths via symlink layout.
