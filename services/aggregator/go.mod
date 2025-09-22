// v3
// file: go.mod
module NRG-CHAMP/aggregator

go 1.22

require (
	github.com/segmentio/kafka-go v0.4.47
	github.com/nrg-champ/circuitbreaker v0.0.0-00010101000000-000000000000
)

replace github.com/nrg-champ/circuitbreaker => ../circuit_breaker
