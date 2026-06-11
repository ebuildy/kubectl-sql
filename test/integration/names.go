//go:build integration

package integration

import (
	"fmt"
	"math/rand"
)

var adjectives = []string{
	"amber", "bold", "crisp", "deft", "eager",
	"fleet", "glad", "hardy", "icy", "jolly",
}

var nouns = []string{
	"crane", "drift", "ember", "flare", "grove",
	"haven", "inlet", "jetty", "knoll", "ledge",
}

var apps = []string{
	"nginx", "redis", "postgres", "mongo", "apache",
	"mysql", "rabbitmq", "elasticsearch", "kafka", "cassandra",
}

var rng = rand.New(rand.NewSource(42))

// randomName generates a random name in the format "adjective-noun-hexsuffix", e.g. "glad-ember-1a2b".
func randomName() string {
	adj := adjectives[rng.Intn(len(adjectives))]
	noun := nouns[rng.Intn(len(nouns))]
	suffix := fmt.Sprintf("%04x", rng.Intn(0x10000))
	return fmt.Sprintf("%s-%s-%s", adj, noun, suffix)
}

var counterRandomAppName = 0

// randomAppName returns a random app name from the apps list, used for seeding pod names and labels.
func randomAppName() string {
	counterRandomAppName++
	return apps[counterRandomAppName%len(apps)]
}
