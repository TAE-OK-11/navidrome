# HTTP/3 deployment notes

Enable HTTP/3 with TLS and publish the configured Navidrome port over both TCP and UDP:

```env
ND_ENABLEHTTP3=true
ND_TLSCERT=/path/to/fullchain.pem
ND_TLSKEY=/path/to/privkey.pem
```

```yaml
ports:
  - "4533:4533/tcp"
  - "4533:4533/udp"
```

quic-go automatically uses Linux Generic Segmentation Offload when the kernel supports it and automatically falls back when it does not. Path MTU discovery is enabled.

For high-throughput QUIC on Linux, raise the kernel maximum UDP socket buffers before starting Navidrome:

```sh
sudo sysctl --system
```

A ready-to-install example is available at `contrib/sysctl/99-navidrome-quic.conf`. Container port publishing does not change the host kernel limits, so apply the settings on the Docker host.
