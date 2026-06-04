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

var rng = rand.New(rand.NewSource(42))

func randomName() string {
	adj := adjectives[rng.Intn(len(adjectives))]
	noun := nouns[rng.Intn(len(nouns))]
	suffix := fmt.Sprintf("%04x", rng.Intn(0x10000))
	return fmt.Sprintf("%s-%s-%s", adj, noun, suffix)
}
