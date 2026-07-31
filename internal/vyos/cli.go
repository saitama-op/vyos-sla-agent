package vyos

import (
	"bytes"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
)

// Controller manages stateful, thread-safe configuration changes to VyOS
type Controller struct {
	mu sync.Mutex
}

// NewController creates a new VyOS CLI controller
func NewController() *Controller {
	return &Controller{}
}

// ExecuteTransaction wraps a series of configuration commands in a single vbash session
func (c *Controller) ExecuteTransaction(commands []string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	slog.Debug("Starting VyOS configuration transaction")

	// 1. Build a standard VyOS vbash configuration script in memory
	var script bytes.Buffer
	script.WriteString("#!/bin/vbash\n")
	
	// Source the official VyOS script template (this enables the 'set', 'delete', and 'commit' commands)
	script.WriteString("source /opt/vyatta/etc/functions/script-template\n")
	
	// Enter config mode (this automatically sets up the session lock and environment variables)
	script.WriteString("configure\n") 

	// 2. Inject the dynamically configured SLA commands from your YAML
	for _, cmd := range commands {
		script.WriteString(cmd + "\n")
	}

	// 3. Commit and exit cleanly (this releases the session lock)
	script.WriteString("commit\n")
	script.WriteString("exit\n")

	// 4. Execute the script in a single VyOS shell process via stdin
	cmd := exec.Command("vbash", "-s")
	cmd.Stdin = &script
	
	// We capture BOTH stdout and stderr now. If you ever have a typo in your YAML syntax,
	// the exact VyOS error message will show up perfectly in your journalctl logs.
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("vbash transaction failed: %w | stdout: %s | stderr: %s", 
			err, 
			strings.TrimSpace(stdout.String()), 
			strings.TrimSpace(stderr.String()),
		)
	}

	slog.Info("VyOS configuration transaction committed successfully", "commands_executed", len(commands))
	return nil
}
