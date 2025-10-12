// v2
// go.mod
module nrgchamp/gamification

go 1.23.0

require (
	github.com/nrg-champ/circuitbreaker v0.0.0
	github.com/segmentio/kafka-go v0.4.47
)

require (
	github.com/klauspost/compress v1.15.9 // indirect
	github.com/pierrec/lz4/v4 v4.1.15 // indirect
)

replace github.com/nrg-champ/circuitbreaker => ../../circuit_breaker
