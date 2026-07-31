package probe

import (
	"context"
	"fmt"
	"net"
	"os"
	"syscall"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/sys/unix"
)

func getInterfaceIPv4(ifaceName string) (string, error) {
        iface, err := net.InterfaceByName(ifaceName)
        if err != nil {
                return "", fmt.Errorf("could not find interface %s: %w", ifaceName, err)
        }

        addrs, err := iface.Addrs()
        if err != nil {
                return "", fmt.Errorf("could not get addresses for %s: %w", ifaceName, err)
        }

        for _, addr := range addrs {
                var ip net.IP
                switch v := addr.(type) {
                case *net.IPNet:
                        ip = v.IP
                case *net.IPAddr:
                        ip = v.IP
                }

                // Ensure it is a valid IPv4 address and not a loopback
                if ip == nil || ip.IsLoopback() {
                        continue
                }
                ip = ip.To4()
                if ip != nil {
                        return ip.String(), nil
                }
        }
        return "", fmt.Errorf("no IPv4 address found on interface %s", ifaceName)
}

func CreateInterfaceBoundICMPConn(ifaceName string) (net.PacketConn, error) {
        // --- NEW: Fetch the specific IP of the interface ---
        srcIP, err := getInterfaceIPv4(ifaceName)
        if err != nil {
                return nil, fmt.Errorf("failed to resolve source IP: %w", err)
        }
        // ---------------------------------------------------

        lc := net.ListenConfig{
                Control: func(network, address string, c syscall.RawConn) error {
                        var bindErr error
                        err := c.Control(func(fd uintptr) {
                                bindErr = unix.SetsockoptString(int(fd), unix.SOL_SOCKET, unix.SO_BINDTODEVICE, ifaceName)
                        })
                        if err != nil {
                                return err
                        }
                        return bindErr
                },
        }

        // --- CHANGED: Use srcIP instead of "0.0.0.0" ---
        pconn, err := lc.ListenPacket(context.Background(), "ip4:icmp", srcIP)
        if err != nil {
                return nil, fmt.Errorf("failed to listen on raw icmp for %s (%s): %w", ifaceName, srcIP, err)
        }

        // Wrap the packet connection to access IPv4-specific socket options
        ipv4Conn := ipv4.NewPacketConn(pconn)
        
        // 0xb8 (184) is DSCP EF (Expedited Forwarding) for critical control traffic
        if err := ipv4Conn.SetTOS(0xb8); err != nil {
                pconn.Close()
                return nil, fmt.Errorf("failed to set TOS on icmp socket for %s: %w", ifaceName, err)
        }

        return pconn, nil
}



func MeasureRTT(ifaceName string, targetIP string, seq int) (time.Duration, error) {
	// 1. Open a dedicated socket for this specific probe to prevent goroutine packet stealing
	conn, err := CreateInterfaceBoundICMPConn(ifaceName)
	if err != nil {
		return 0, err
	}
	defer conn.Close()

	pid := os.Getpid() & 0xffff
	wm := icmp.Message{
		Type: ipv4.ICMPTypeEcho, Code: 0,
		Body: &icmp.Echo{
			ID: pid, Seq: seq,
			Data: []byte("vyos-sla-agent"),
		},
	}
	wb, err := wm.Marshal(nil)
	if err != nil {
		return 0, err
	}

	dst, err := net.ResolveIPAddr("ip4", targetIP)
	if err != nil {
		return 0, err
	}

	start := time.Now()
	if _, err := conn.WriteTo(wb, dst); err != nil {
		return 0, err
	}

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))

	// 2. Loop to discard background ICMP noise on the interface until we find our exact packet
	for {
		rb := make([]byte, 1500)
		n, peer, err := conn.ReadFrom(rb)
		if err != nil {
			return 0, err // Timeout or read error
		}

		// Ignore replies that didn't come from our specific target IP
		if peer.String() != dst.String() {
			continue
		}

		rm, err := icmp.ParseMessage(ipv4.ICMPTypeEcho.Protocol(), rb[:n])
		if err != nil {
			continue
		}

		// Ignore non-Echo-Replies and verify the ID and Sequence match exactly
		if rm.Type == ipv4.ICMPTypeEchoReply {
			if echo, ok := rm.Body.(*icmp.Echo); ok {
				if echo.ID == pid && echo.Seq == seq {
					// This is our exact packet!
					return time.Since(start), nil
				}
			}
		}
	}
}


