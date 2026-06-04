//go:build integration

package integration

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/cucumber/godog"
)

type testContext struct {
	stdout          string
	stderr          string
	exitCode        int
	pickedNamespace string
}

func (tc *testContext) iRunKubectlSql(query string) error {
	binary := "../../bin/kubectl-sql"
	cmd := exec.Command(binary, "--kubeconfig", envKubeconfig, query)
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
	sc.Step(`^the output has at least (\d+) rows$`, tc.theOutputHasAtLeastRows)
	sc.Step(`^the output has at most (\d+) rows$`, tc.theOutputHasAtMostRows)
	sc.Step(`^the output has between (\d+) and (\d+) rows$`, tc.theOutputHasBetweenAndRows)
	sc.Step(`^the exit code is (\d+)$`, tc.theExitCodeIs)
	sc.Step(`^I pick a random fixture namespace$`, tc.iPickARandomFixtureNamespace)
}
