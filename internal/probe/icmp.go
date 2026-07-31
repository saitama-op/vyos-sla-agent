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

func CreateInterfaceBoundICMPConn(ifaceName string) (net.PacketConn, error) {
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

	pconn, err := lc.ListenPacket(context.Background(), "ip4:icmp", "0.0.0.0")
	if err != nil {
		return nil, fmt.Errorf("failed to listen on raw icmp for %s: %w", ifaceName, err)
	}
	
	// Simply return the standard net.PacketConn
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


