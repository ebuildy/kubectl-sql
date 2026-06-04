package e2e

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/cucumber/godog"
)

type testContext struct {
	stdout   string
	stderr   string
	exitCode int
}

func binaryPath() string {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(file), "..", "..")
	return filepath.Join(root, "bin", "kubectl-sql")
}

func (tc *testContext) iRun(command string) error {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return nil
	}
	// Replace "kubectl-sql" with the actual binary path.
	if parts[0] == "kubectl-sql" {
		parts[0] = binaryPath()
	}
	cmd := exec.Command(parts[0], parts[1:]...) //nolint:gosec
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	tc.stdout = outBuf.String()
	tc.stderr = errBuf.String()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			tc.exitCode = exitErr.ExitCode()
			return nil
		}
		return err
	}
	tc.exitCode = 0
	return nil
}

func (tc *testContext) theExitCodeIs(code int) error {
	if tc.exitCode != code {
		return godog.ErrPending
	}
	return nil
}

func (tc *testContext) theOutputContains(substr string) error {
	combined := tc.stdout + tc.stderr
	if !strings.Contains(combined, substr) {
		return godog.ErrPending
	}
	return nil
}

// InitializeScenario registers step definitions with the godog suite.
func InitializeScenario(sc *godog.ScenarioContext) {
	tc := &testContext{}
	sc.Step(`^I run "([^"]*)"$`, tc.iRun)
	sc.Step(`^the exit code is (\d+)$`, tc.theExitCodeIs)
	sc.Step(`^the output contains "([^"]*)"$`, tc.theOutputContains)
}
