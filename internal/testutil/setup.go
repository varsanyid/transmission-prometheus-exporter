package testutil

import (
	"testing"
	
	"github.com/leanovate/gopter"
)

// PropertyTestConfig holds configuration for property-based tests
type PropertyTestConfig struct {
	MinSuccessfulTests int
	MaxDiscardRatio    float64
	Workers            int
}

// DefaultPropertyTestConfig returns default configuration for property tests
func DefaultPropertyTestConfig() *PropertyTestConfig {
	return &PropertyTestConfig{
		MinSuccessfulTests: 100, // Run minimum 100 iterations as specified in design
		MaxDiscardRatio:    5.0,
		Workers:            1,
	}
}

// RunPropertyTest runs a property-based test with the given configuration
func RunPropertyTest(t *testing.T, testName string, property gopter.Prop, config *PropertyTestConfig) {
	if config == nil {
		config = DefaultPropertyTestConfig()
	}
	
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = config.MinSuccessfulTests
	parameters.MaxDiscardRatio = config.MaxDiscardRatio
	parameters.Workers = config.Workers
	
	properties := gopter.NewProperties(parameters)
	properties.Property(testName, property)
	
	properties.TestingRun(t)
}