//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/cucumber/godog"
	"github.com/itchyny/gojq"
)

type testContext struct {
	stdout          string
	stderr          string
	exitCode        int
	pickedNamespace string
}

func (tc *testContext) iRunKubectlSql(query string) error {
	// Default to JSON output so steps can use JQ assertions.
	return tc.runBinary("--output", "json", query)
}

func (tc *testContext) runBinary(args ...string) error {
	const deadline = 5 * time.Second
	binary := "../../bin/kubectl-sql"
	baseArgs := []string{"--kubeconfig", envKubeconfig, "--timeout", "10s"}
	ctx, cancel := context.WithTimeout(context.Background(), deadline)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, append(baseArgs, args...)...)
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

func (tc *testContext) theOutputHasAtLeastRows(minRows int) error {
	count := countDataRows(tc.stdout)
	if count < minRows {
		return fmt.Errorf("expected at least %d rows, got %d\noutput:\n%s", minRows, count, tc.stdout)
	}
	return nil
}

func (tc *testContext) theOutputHasAtMostRows(maxRows int) error {
	count := countDataRows(tc.stdout)
	if count > maxRows {
		return fmt.Errorf("expected at most %d rows, got %d\noutput:\n%s", maxRows, count, tc.stdout)
	}
	return nil
}

func (tc *testContext) theOutputHasBetweenAndRows(minRows, maxRows int) error {
	count := countDataRows(tc.stdout)
	if count < minRows || count > maxRows {
		return fmt.Errorf("expected between %d and %d rows, got %d\noutput:\n%s", minRows, maxRows, count, tc.stdout)
	}
	return nil
}

func (tc *testContext) theExitCodeIs(code int) error {
	if tc.exitCode != code {
		return fmt.Errorf("expected exit code %d, got %d\nstdout: %s\nstderr: %s", code, tc.exitCode, tc.stdout, tc.stderr)
	}
	return nil
}

func (tc *testContext) theExitCodeIsNot(code int) error {
	if tc.exitCode == code {
		return fmt.Errorf("expected exit code != %d\nstdout: %s\nstderr: %s", code, tc.stdout, tc.stderr)
	}
	return nil
}

func (tc *testContext) iPickARandomFixtureNamespace() error {
	if len(envNamespaces) == 0 {
		return fmt.Errorf("no fixture namespaces available")
	}
	tc.pickedNamespace = envNamespaces[0]
	return nil
}

func (tc *testContext) iRunKubectlSqlInNamespace(query string) error {
	query = strings.ReplaceAll(query, "<fixture-namespace>", tc.pickedNamespace)
	return tc.iRunKubectlSql(query)
}

func (tc *testContext) iRunKubectlSqlWithNamespaceFlag(ns, query string) error {
	return tc.runBinary("--output", "json", "--namespace", ns, query)
}

func (tc *testContext) iRunKubectlSqlWithOutputFlag(format, query string) error {
	return tc.runBinary("--output", format, query)
}

func (tc *testContext) iRunKubectlSqlWithWatchFlag(query string) error {
	// Run with a short timeout so the watch stream exits cleanly after seeing initial events.
	return tc.runBinary("--watch", "--timeout", "3s", "--output", "json", query)
}

func (tc *testContext) iRunKubectlSqlVerbose(flag, query string) error {
	return tc.runBinary(flag, "--output", "json", query)
}

func (tc *testContext) theStderrContains(s string) error {
	if strings.Contains(tc.stderr, s) {
		return nil
	}
	return fmt.Errorf("expected stderr to contain %q\nstderr:\n%s", s, tc.stderr)
}

func (tc *testContext) theStderrIsEmpty() error {
	if strings.TrimSpace(tc.stderr) != "" {
		return fmt.Errorf("expected empty stderr, got:\n%s", tc.stderr)
	}
	return nil
}

// iPipeQueryToKubectlSql runs the binary with no positional query, feeding the
// query on stdin. With non-TTY stdin the REPL enters batch mode.
func (tc *testContext) iPipeQueryToKubectlSql(query string) error {
	const deadline = 5 * time.Second
	binary := "../../bin/kubectl-sql"
	args := []string{"--kubeconfig", envKubeconfig, "--timeout", "10s", "--output", "json"}
	ctx, cancel := context.WithTimeout(context.Background(), deadline)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Env = append(os.Environ(), "TERM=dumb")
	cmd.Stdin = strings.NewReader(query + "\n")
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

func (tc *testContext) theOutputContains(s string) error {
	if strings.Contains(tc.stdout, s) || strings.Contains(tc.stderr, s) {
		return nil
	}
	return fmt.Errorf("expected output to contain %q\nstdout:\n%s\nstderr:\n%s", s, tc.stdout, tc.stderr)
}

func (tc *testContext) theOutputDoesNotContain(s string) error {
	if strings.Contains(tc.stdout, s) || strings.Contains(tc.stderr, s) {
		return fmt.Errorf("expected output NOT to contain %q\nstdout:\n%s\nstderr:\n%s", s, tc.stdout, tc.stderr)
	}
	return nil
}

// theOutputProducesJQ runs a JQ query against the JSON stdout and asserts it
// produces at least one truthy (non-null, non-false) result.
// Example step: `the output produces JQ "[.[] | select(.\"pods.name\" != null)] | length > 0"`
func (tc *testContext) theOutputProducesJQ(jqExpr string) error {
	var input interface{}
	if err := json.Unmarshal([]byte(tc.stdout), &input); err != nil {
		return fmt.Errorf("output is not valid JSON: %w\noutput:\n%s", err, tc.stdout)
	}

	q, err := gojq.Parse(jqExpr)
	if err != nil {
		return fmt.Errorf("invalid JQ expression %q: %w", jqExpr, err)
	}

	iter := q.Run(input)
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if err, ok := v.(error); ok {
			return fmt.Errorf("JQ error for %q: %w\noutput:\n%s", jqExpr, err, tc.stdout)
		}
		// Any non-null, non-false result satisfies the assertion.
		if v != nil && v != false {
			return nil
		}
	}
	return fmt.Errorf("JQ expression %q produced no truthy result\noutput:\n%s", jqExpr, tc.stdout)
}

// countDataRows counts data rows in table output, excluding the header row and separator lines.
// The table format is:
//
//	+-------+      <- separator
//	| name  |      <- header (skipped)
//	+-------+      <- separator
//	| foo   |      <- data row (counted)
//	+-------+      <- separator
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
		// Skip separator lines (lines composed only of '+', '-', ' ')
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
		// First non-separator | row is the header
		if !headerSeen {
			headerSeen = true
			continue
		}
		count++
	}
	return count
}

func InitializeScenario(sc *godog.ScenarioContext) {
	tc := &testContext{}

	sc.Step(`^I run kubectl-sql "([^"]*)" against the envtest cluster$`, tc.iRunKubectlSql)
	sc.Step(`^I run kubectl-sql with namespace query "([^"]*)" against the envtest cluster$`, tc.iRunKubectlSqlInNamespace)
	sc.Step(`^I run kubectl-sql --namespace "([^"]*)" with query "([^"]*)" against the envtest cluster$`, tc.iRunKubectlSqlWithNamespaceFlag)
	sc.Step(`^I run kubectl-sql --output "([^"]*)" with query "([^"]*)" against the envtest cluster$`, tc.iRunKubectlSqlWithOutputFlag)
	sc.Step(`^the output has at least (\d+) rows$`, tc.theOutputHasAtLeastRows)
	sc.Step(`^the output has at most (\d+) rows$`, tc.theOutputHasAtMostRows)
	sc.Step(`^the output has between (\d+) and (\d+) rows$`, tc.theOutputHasBetweenAndRows)
	sc.Step(`^the exit code is (\d+)$`, tc.theExitCodeIs)
	sc.Step(`^the exit code is not (\d+)$`, tc.theExitCodeIsNot)
	sc.Step(`^the output contains "([^"]*)"$`, tc.theOutputContains)
	sc.Step(`^the output does not contain "([^"]*)"$`, tc.theOutputDoesNotContain)
	sc.Step(`^the output produces JQ "([^"]*)"$`, tc.theOutputProducesJQ)
	sc.Step(`^I run kubectl-sql --watch "([^"]*)" against the envtest cluster$`, tc.iRunKubectlSqlWithWatchFlag)
	sc.Step(`^I pipe "([^"]*)" to kubectl-sql against the envtest cluster$`, tc.iPipeQueryToKubectlSql)
	sc.Step(`^I run kubectl-sql ([\-v]+) with query "([^"]*)" against the envtest cluster$`, tc.iRunKubectlSqlVerbose)
	sc.Step(`^the stderr contains "([^"]*)"$`, tc.theStderrContains)
	sc.Step(`^the stderr is empty$`, tc.theStderrIsEmpty)
	sc.Step(`^I pick a random fixture namespace$`, tc.iPickARandomFixtureNamespace)
}
