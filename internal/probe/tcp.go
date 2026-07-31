package probe

import (
	"net"
	"syscall"
	"time"
)

// SendTCPProbe performs a TCP handshake to test application-layer connectivity
func SendTCPProbe(ifaceName, target string) (time.Duration, error) {
	dialer := &net.Dialer{
		Timeout: 2 * time.Second,
		Control: func(network, address string, c syscall.RawConn) error {
			var sockErr error
			err := c.Control(func(fd uintptr) {
				// Bind to specific interface (eth0, eth1, etc.)
				if ifaceName != "" {
					sockErr = syscall.SetsockoptString(int(fd), syscall.SOL_SOCKET, syscall.SO_BINDTODEVICE, ifaceName)
				}
				// Inject DSCP EF (tos 0xb8)
				syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IP, syscall.IP_TOS, 0xb8)
			})
			if err != nil {
				return err
			}
			return sockErr
		},
	}

	start := time.Now()
	conn, err := dialer.Dial("tcp", target)
	if err != nil {
		return 0, err
	}
	conn.Close()
	return time.Since(start), nil
}
