# Outgoing Port Discovery Tool

[![Go Reference](https://pkg.go.dev/badge/github.com/database64128/opdt-go.svg)](https://pkg.go.dev/github.com/database64128/opdt-go)
[![Test](https://github.com/database64128/opdt-go/actions/workflows/test.yml/badge.svg)](https://github.com/database64128/opdt-go/actions/workflows/test.yml)
[![Release](https://github.com/database64128/opdt-go/actions/workflows/release.yml/badge.svg)](https://github.com/database64128/opdt-go/actions/workflows/release.yml)
[![opdt-go AUR package](https://img.shields.io/aur/version/opdt-go?label=opdt-go)](https://aur.archlinux.org/packages/opdt-go)
[![opdt-go-git AUR package](https://img.shields.io/aur/version/opdt-go-git?label=opdt-go-git)](https://aur.archlinux.org/packages/opdt-go-git)

Discover the actual outgoing address and port when behind a NAT.

## Features

- Designed for easy and secure self-hosting.
- XChaCha20-Poly1305 AEAD.

## Usage

To get started, generate a PSK, edit the server configuration, and start the server:

```bash
openssl rand -base64 32
sudo nano /etc/opdt-go/config.json
sudo systemctl enable --now opdt-go
```

Run the program in client mode to discover the client address and port:

```bash
opdt-go -client '[2001:db8:bd63:362c:2071:a0f6:827:ab6a]:20220' -clientBind ':10128' -clientPSK 'XbQZKDJTbbhuSwF0muQx6L9swsAmf0VOYIApri7nHUQ='
```

## License

[AGPLv3](LICENSE)
