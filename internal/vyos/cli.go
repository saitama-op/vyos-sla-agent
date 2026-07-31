package vyos

import (
	"bytes"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
)

const WrapperPath = "/opt/vyatta/sbin/vyatta-cfg-cmd-wrapper"

// Controller manages stateful, thread-safe configuration changes to VyOS
type Controller struct {
	// mu ensures only one configuration transaction occurs at a time,
	// preventing VyOS config lock contention.
	mu sync.Mutex
}

// NewController creates a new VyOS CLI controller
func NewController() *Controller {
	return &Controller{}
}

// runCommand executes the raw wrapper command and captures stderr for debugging
func (c *Controller) runCommand(args ...string) error {
	cmd := exec.Command(WrapperPath, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w (stderr: %s)", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// ExecuteTransaction wraps a series of configuration commands in a safe session
func (c *Controller) ExecuteTransaction(commands []string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	slog.Debug("Starting VyOS configuration transaction")

	if err := c.runCommand("begin"); err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	// Track success to handle automated rollbacks
	success := false
	defer func() {
		if !success {
			slog.Warn("Transaction failed, issuing discard to rollback candidate config")
			_ = c.runCommand("discard") // Ignore discard errors on rollback
		}
		// Always end the session to release the VyOS configuration lock
		if err := c.runCommand("end"); err != nil {
			slog.Error("Failed to cleanly end VyOS transaction", "error", err)
		}
	}()

	// Execute each command sequentially
	for _, cmdStr := range commands {
		// e.g., "set protocols static route 0.0.0.0/0 next-hop 1.1.1.1 disable"
		args := strings.Fields(cmdStr) 
		if err := c.runCommand(args...); err != nil {
			return fmt.Errorf("failed on command '%s': %w", cmdStr, err)
		}
	}

	slog.Debug("Committing VyOS transaction")
	if err := c.runCommand("commit"); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	success = true
	slog.Info("VyOS configuration transaction committed successfully", "commands_executed", len(commands))
	return nil
}

// DisableInterface is a convenience method for turning down an interface directly
func (c *Controller) DisableInterface(iface string) error {
	slog.Warn("SLA Action: Disabling interface", "interface", iface)
	return c.ExecuteTransaction([]string{
		fmt.Sprintf("set interfaces ethernet %s disable", iface),
	})
}

// EnableInterface is a convenience method for turning up an interface directly
func (c *Controller) EnableInterface(iface string) error {
	slog.Info("SLA Action: Enabling interface", "interface", iface)
	return c.ExecuteTransaction([]string{
		fmt.Sprintf("delete interfaces ethernet %s disable", iface),
	})
}
