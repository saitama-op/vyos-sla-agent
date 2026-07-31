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

func MeasureRTT(conn net.PacketConn, targetIP string, seq int) (time.Duration, error) {
	wm := icmp.Message{
		Type: ipv4.ICMPTypeEcho, Code: 0,
		Body: &icmp.Echo{
			ID: os.Getpid() & 0xffff, Seq: seq,
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
	rb := make([]byte, 1500)
	n, _, err := conn.ReadFrom(rb)
	if err != nil {
		return 0, err
	}
	
	duration := time.Since(start)

	rm, err := icmp.ParseMessage(ipv4.ICMPTypeEcho.Protocol(), rb[:n])
	if err != nil {
		return 0, err
	}
	if rm.Type == ipv4.ICMPTypeEchoReply {
		return duration, nil
	}
	return 0, fmt.Errorf("invalid reply")
}
