//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

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
	if parts[0] == "kubectl-sql" {
		parts[0] = binaryPath()
	}
	return tc.runCommand(parts[0], parts[1:]...)
}

func (tc *testContext) runCommand(binary string, args ...string) error {
	const deadline = 15 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), deadline)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, args...) //nolint:gosec
	cmd.Env = append(os.Environ(), "TERM=dumb")
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	tc.stdout = outBuf.String()
	tc.stderr = errBuf.String()
	if ctx.Err() != nil {
		return fmt.Errorf("binary timed out after %s\nstdout: %s\nstderr: %s", deadline, tc.stdout, tc.stderr)
	}
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
		return fmt.Errorf("expected exit code %d, got %d\nstdout: %s\nstderr: %s", code, tc.exitCode, tc.stdout, tc.stderr)
	}
	return nil
}

func (tc *testContext) theOutputContains(substr string) error {
	combined := tc.stdout + tc.stderr
	if !strings.Contains(combined, substr) {
		return fmt.Errorf("expected output to contain %q\noutput:\n%s", substr, combined)
	}
	return nil
}

func (tc *testContext) theExitCodeIsNot(code int) error {
	if tc.exitCode == code {
		return fmt.Errorf("expected exit code != %d but got %d", code, tc.exitCode)
	}
	return nil
}

func (tc *testContext) runBinary(args ...string) error {
	return tc.runCommand(binaryPath(), args...)
}

// envtestKubeconfig returns the kubeconfig to use for envtest scenarios.
// Returns ("", false) when no envtest cluster is available; callers should skip.
func envtestKubeconfig() (string, bool) {
	kc := os.Getenv("ENVTEST_KUBECONFIG")
	return kc, kc != ""
}

func (tc *testContext) iRunKubectlSqlAgainstEnvtest(query string) error {
	kc, ok := envtestKubeconfig()
	if !ok {
		return godog.ErrSkip
	}
	return tc.runBinary("--kubeconfig", kc, query)
}

func (tc *testContext) iRunKubectlSqlWithNamespaceQueryAgainstEnvtest(query string) error {
	kc, ok := envtestKubeconfig()
	if !ok {
		return godog.ErrSkip
	}
	return tc.runBinary("--kubeconfig", kc, query)
}

func (tc *testContext) iRunKubectlSqlWithNamespaceFlagAgainstEnvtest(ns, query string) error {
	kc, ok := envtestKubeconfig()
	if !ok {
		return godog.ErrSkip
	}
	return tc.runBinary("--kubeconfig", kc, "--namespace", ns, query)
}

func (tc *testContext) iPickARandomFixtureNamespace() error {
	_, ok := envtestKubeconfig()
	if !ok {
		return godog.ErrSkip
	}
	return nil
}

func (tc *testContext) theOutputHasAtLeastRows(min int) error {
	count := countDataRows(tc.stdout)
	if count < min {
		return fmt.Errorf("expected at least %d rows, got %d\noutput:\n%s", min, count, tc.stdout)
	}
	return nil
}

func (tc *testContext) theOutputHasAtMostRows(max int) error {
	count := countDataRows(tc.stdout)
	if count > max {
		return fmt.Errorf("expected at most %d rows, got %d\noutput:\n%s", max, count, tc.stdout)
	}
	return nil
}

func (tc *testContext) theOutputHasBetweenAndRows(min, max int) error {
	count := countDataRows(tc.stdout)
	if count < min || count > max {
		return fmt.Errorf("expected between %d and %d rows, got %d\noutput:\n%s", min, max, count, tc.stdout)
	}
	return nil
}

func countDataRows(output string) int {
	count := 0
	headerSeen := false
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "|") && !strings.HasPrefix(line, "+") {
			continue
		}
		isSeparator := true
		for _, ch := range line {
			if ch != '|' && ch != '-' && ch != '+' && ch != ' ' {
				isSeparator = false
				break
			}
		}
		if isSeparator {
			continue
		}
		if !headerSeen {
			headerSeen = true
			continue
		}
		count++
	}
	return count
}

// InitializeScenario registers step definitions with the godog suite.
func InitializeScenario(sc *godog.ScenarioContext) {
	tc := &testContext{}
	sc.Step(`^I run "([^"]*)"$`, tc.iRun)
	sc.Step(`^the exit code is (\d+)$`, tc.theExitCodeIs)
	sc.Step(`^the exit code is not (\d+)$`, tc.theExitCodeIsNot)
	sc.Step(`^the output contains "([^"]*)"$`, tc.theOutputContains)
	sc.Step(`^I run kubectl-sql "([^"]*)" against the envtest cluster$`, tc.iRunKubectlSqlAgainstEnvtest)
	sc.Step(`^I run kubectl-sql with namespace query "([^"]*)" against the envtest cluster$`, tc.iRunKubectlSqlWithNamespaceQueryAgainstEnvtest)
	sc.Step(`^I run kubectl-sql --namespace "([^"]*)" with query "([^"]*)" against the envtest cluster$`, tc.iRunKubectlSqlWithNamespaceFlagAgainstEnvtest)
	sc.Step(`^the output has at least (\d+) rows$`, tc.theOutputHasAtLeastRows)
	sc.Step(`^the output has at most (\d+) rows$`, tc.theOutputHasAtMostRows)
	sc.Step(`^the output has between (\d+) and (\d+) rows$`, tc.theOutputHasBetweenAndRows)
	sc.Step(`^I pick a random fixture namespace$`, tc.iPickARandomFixtureNamespace)
}
