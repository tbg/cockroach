package crdbnemesis

import (
	"context"
	"math/rand"
)

type ExampleStepper struct {
	AtLeastV21Dot2MixedSupporter
}

func (opts exampleStepperOptions) Chaos() bool {
	return opts.numTables > 1
}

func (es *ExampleStepper) RandOptions(*rand.Rand) Action {
	return nil
}

func (es *ExampleStepper) Run(ctx context.Context, fataler Fataler, instance Action, c Cluster) {
	panic("implement me")
}

func (es *ExampleStepper) Name() string { return "example" }

func (es *ExampleStepper) Owner() Owner { return OwnerKV }
