---
sidebar_label: Raspberry Pi first boot
sidebar_position: 5
---

# Raspberry Pi first boot

This guide prepares a Raspberry Pi with Ubuntu Server using cloud-init, then
connects to it over SSH. It covers host setup only; it does not install agent
workers or add capabilities beyond the current Ember Compose deployment.

## Requirements

- Raspberry Pi supported by the Ubuntu Server ARM64 image.
- microSD card or USB storage device large enough for the image.
- Linux workstation, `xz-utils`, and permission to write the target block device.
- Ubuntu Server ARM64 image, for example
  `ubuntu-26.04.1-preinstalled-server-arm64+raspi.img.xz`.
- Your SSH public key(s).

## Write the image

Identify the target device before writing:

```sh
lsblk -o NAME,SIZE,MODEL,TRAN,MOUNTPOINTS
```

The example below uses `/dev/sda`. **Replace it with the whole-disk device for
your card** (not a partition such as `/dev/sda1`). Writing the wrong device
destroys its data. Unmount any partitions on the target first.

```sh
xzcat ubuntu-26.04.1-preinstalled-server-arm64+raspi.img.xz |
  sudo dd of=/dev/sda bs=4M status=progress conv=fsync
sync
```

Wait for `dd` and `sync` to finish before removing the media.

## Configure cloud-init

Mount the image's boot partition and edit its `user-data` file:

```sh
sudo mkdir -p /mnt/pi-boot
sudo mount /dev/sda1 /mnt/pi-boot
sudoedit /mnt/pi-boot/user-data
```

The exact partition name may differ (for example, `/dev/mmcblk0p1`). Confirm it
with `lsblk`. Preserve the existing `#cloud-config` header and YAML structure.
Set a hostname, add your own public keys, and disable SSH password login. For
example, update or add these top-level keys:

```yaml
hostname: ember-poc
ssh_pwauth: false
ssh_authorized_keys:
  - ssh-ed25519 REPLACE_WITH_YOUR_PUBLIC_KEY workstation
```

Use the complete contents of your public key file (typically `~/.ssh/id_ed25519.pub`).
Do not put a private key on the image. Do not copy someone else's keys into your
device configuration.

The stock image may configure an initial `ubuntu` account and a temporary
password. If cloud-init is configured to require a password change, complete
that change on first login; do not leave a default password in place. Keep
password-based SSH disabled.

Save the file, then flush and unmount the boot partition:

```sh
sync
sudo umount /mnt/pi-boot
```

## First boot and SSH

Insert the media into the Pi, connect Ethernet (or configure Wi-Fi through the
image's supported cloud-init settings), and power it on. Allow a few minutes
for first-boot cloud-init. Find the device through your router's DHCP leases or
another trusted local discovery method, then connect:

```sh
ssh ubuntu@<pi-ip-address>
```

If you configured local mDNS, try `ssh ubuntu@ember-poc.local`; setting a
hostname alone does not guarantee that `.local` discovery works. Verify the
host and update packages:

```sh
hostnamectl
sudo apt update
sudo apt full-upgrade -y
sudo reboot
```

After reboot, continue with the [Ansible host provisioning and Compose
deployment guide](operations/README.md#ansible-host-provisioning). The current
Ember API has no authentication, so keep its frontend loopback-bound and do not
expose it directly to an untrusted network; see [deployment](deployment.md).
