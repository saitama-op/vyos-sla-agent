# vyos-sla-agent

A high-performance, ultra-lightweight SD-WAN SLA monitoring and failover agent for VyOS, written in Go. 

`vyos-sla-agent` replaces legacy tools like SmokePing or heavy Perl/Python scripts with a single, statically compiled binary. It actively monitors multiple WAN links using concurrent, interface-bound ICMP probes, evaluates connection health against configurable thresholds (Latency, Packet Loss, and RFC 3550 Jitter), and dynamically updates the VyOS routing table when an SLA is breached.

## Features

- **Zero External Dependencies:** No need for `ping`, `fping`, Perl, or Python. Uses raw sockets (`SO_BINDTODEVICE`) natively in Go.
- **SD-WAN Failover:** Automatically disables/enables routes via the VyOS CLI wrapper when thresholds are crossed.
- **High Concurrency:** Probes all WAN interfaces and targets simultaneously using goroutines.
- **Advanced Metrics:** Calculates RFC 3550 standard Jitter and 95th percentile moving averages.
- **Hysteresis State Machine:** Prevents route flapping (e.g., requires 5 bad cycles to mark `DOWN`, 20 good cycles to mark `UP`).
- **Prometheus Exporter:** Native high-cardinality metrics (`sla_latency_ms`, `sla_loss_percent`, `sla_state`).
- **REST API:** Built-in HTTP endpoints for external health checks and status queries.
- **Tiny Footprint:** Minimal CPU usage and < 20MB RAM, compiled as a static binary.

## Architecture
The agent runs an evaluation cycle every `X` seconds (defined in the config). For each WAN, it concurrently pings all targets, calculates rolling metrics over a defined sample size, and evaluates the results against the configured thresholds. If a threshold is breached, the agent safely wraps VyOS `begin`, `set...`, `commit`, and `end` commands in a lock-guarded transaction.


## Installation & Build

Build the static binary for VyOS (Linux AMD64) using the provided Makefile. The build process uses `CGO_ENABLED=0` and strips debugging symbols to keep the binary small.

```bash
git clone [https://github.com/saitama-op/vyos-sla-agent.git](https://github.com/saitama-op/vyos-sla-agent.git)
cd vyos-sla-agent
make
```

## Active SD-WAN Failover (VyOS CLI Integration)

The agent natively integrates with the VyOS configuration shell (`/opt/vyatta/sbin/vyatta-cfg-cmd-wrapper`). Instead of hardcoding failover logic, you can define an array >

The agent automatically wraps these commands in a safe `begin`, `commit`, `end` transaction. If a command fails to apply, the agent will issue a `discard` to prevent confi>

```yaml
    on_down:
      - "set protocols static route 0.0.0.0/0 next-hop 10.0.0.1 disable"
    on_up:
      - "delete protocols static route 0.0.0.0/0 next-hop 10.0.0.1 disable"
```
