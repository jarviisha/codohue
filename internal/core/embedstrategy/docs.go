// Package embedstrategy defines the contract every embedding implementation
// honours. The V1 deterministic Go strategy and any future external-LLM
// strategy implement the same Strategy interface, registering themselves
// against the package-level Registry from init().
package embedstrategy
