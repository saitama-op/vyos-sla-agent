# vyos-sla-agent 

A high-performance, ultra-lightweight SD-WAN SLA monitoring and failover agent for VyOS, written in Go.

`vyos-sla-agent` replaces legacy tools like SmokePing or heavy Perl/Python scripts with a single, statically compiled binary. It actively monitors multiple WAN links using concurrent, interface-bound **ICMP and TCP probes**, evaluates connection health against configurable thresholds (Latency, Packet Loss, and RFC 3550 Jitter), and dynamically updates the VyOS routing table when an SLA is breached.

## Features

- **Zero External Dependencies:** No need for `ping`, `fping`, Perl, or Python. Uses raw sockets (`SO_BINDTODEVICE`) natively in Go.
- **Dual-Protocol Probing:** Concurrently sends ICMP Echo Requests and TCP SYN handshakes (e.g., port 443) to detect both network-layer routing issues and application-layer drops (like silent upstream NAT/Firewall failures). 
- **SD-WAN Failover:** Automatically disables/enables routes via the VyOS CLI wrapper when thresholds are crossed.
- **High Concurrency:** Probes all WAN interfaces and targets simultaneously using goroutines.
- **Advanced Metrics:** Calculates RFC 3550 standard Jitter and 95th percentile moving averages.
- **Hysteresis State Machine:** Prevents route flapping (e.g., requires 5 bad cycles to mark `DOWN`, 20 good cycles to mark `UP`).
- **Prometheus Exporter:** Native high-cardinality metrics (`sla_latency_ms`, `sla_loss_percent`, `sla_jitter_ms`, `sla_tcp_latency_ms`, `sla_tcp_loss_percent`, `sla_state`).
- **REST API:** Built-in HTTP endpoints for external health checks and status queries.
- **Tiny Footprint:** Minimal CPU usage and < 20MB RAM, compiled as a static binary.

## Architecture

The agent runs an evaluation cycle every `X` seconds (defined in the config). For each WAN, it concurrently executes ICMP pings and TCP handshakes to all configured targets, calculates rolling metrics over a defined sample size, and evaluates the results against the configured thresholds. 

**Failover Logic:** The agent uses an `OR` logic gate. If *either* the ICMP probes *or* the TCP probes breach the configured latency or packet loss thresholds, the link is penalized. If a threshold is consistently breached, the agent safely wraps VyOS `begin`, `set...`, `commit`, and `end` commands in a lock-guarded transaction.

## Installation & Build

Build the static binary for VyOS (Linux AMD64) using the provided Makefile. The build process uses `CGO_ENABLED=0` and strips debugging symbols to keep the binary small.

```bash
git clone [https://github.com/saitama-op/vyos-sla-agent.git](https://github.com/saitama-op/vyos-sla-agent.git)
cd vyos-sla-agent
make
```

### Deploying on VyOS

To ensure the agent survives VyOS image upgrades, all files must be stored in the `/config` directory. 

**1. Transfer the binary and configuration:**
Create a directory in `/config/user-data/` and copy the compiled binary and `config.yaml` to your router.

```bash
mkdir -p /config/user-data/vyos-sla-agent/configs
# Copy your compiled 'sla-agent' binary and 'config.yaml' into this directory
chmod +x /config/user-data/vyos-sla-agent/sla-agent
```

**2. Update `config.yaml` to include TCP targets:**
Ensure your WAN blocks include the `tcp_targets` array specifying the target port:

```yaml
interval: 2s
wans:
  - name: WAN1
    interface: eth0
    targets:
      - 1.1.1.1
      - 8.8.8.8
    tcp_targets:
      - "1.1.1.1:443"
      - "8.8.8.8:443"
    threshold:
      latency: 250
      latency_samples: 40
      loss: 5
      loss_samples: 40
      jitter: 50
      jitter_samples: 40
```

**3. Create a Systemd Service:**
Create a systemd unit file at `/config/user-data/vyos-sla-agent/vyos-sla-agent.service`:

```ini
[Unit]
Description=VyOS SLA Agent
After=network-online.target vyos-router.service
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=/config/user-data/vyos-sla-agent
ExecStart=/config/user-data/vyos-sla-agent/sla-agent
Restart=always
RestartSec=5
# VyOS configuration commands require root privileges
User=root

[Install]
WantedBy=multi-user.target
```

**4. Persist the Service Across Reboots & Upgrades:**
VyOS clears `/etc/systemd/system/` on image upgrades. To ensure the service is loaded automatically on boot, add the following lines to `/config/scripts/vyos-postconfig-bootup.script`:

```bash
# Link and start the VyOS SLA Agent
ln -sf /config/user-data/vyos-sla-agent/vyos-sla-agent.service /etc/systemd/system/vyos-sla-agent.service
systemctl daemon-reload
systemctl enable --now vyos-sla-agent.service
```

## Active SD-WAN Failover (VyOS CLI Integration)

The agent natively integrates with the VyOS configuration shell (`/opt/vyatta/sbin/vyatta-cfg-cmd-wrapper`). Instead of hardcoding failover logic, you can define an array of VyOS commands in your `config.yaml` to be executed when a link transitions to `DOWN` or `UP`.

The agent automatically wraps these commands in a safe `begin`, `commit`, `end` transaction. If a command fails to apply, the agent will issue a `discard` to prevent configuration lockouts.

```yaml
    on_down:
      - "set protocols static route 0.0.0.0/0 next-hop 10.0.0.1 disable"
    on_up:
      - "delete protocols static route 0.0.0.0/0 next-hop 10.0.0.1 disable"
```

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details. It is completely free to use, modify, and distribute.
