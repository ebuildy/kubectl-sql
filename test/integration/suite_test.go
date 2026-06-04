//go:build integration

package integration

import (
	"testing"

	"github.com/cucumber/godog"
)

func TestFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: InitializeScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"../e2e/features/integration.feature"},
			TestingT: t,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("integration feature tests failed")
	}
}
