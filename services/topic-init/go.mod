// v0
// services/topic-init/go.mod
module NRG-CHAMP/topic-init

go 1.22

require github.com/segmentio/kafka-go v0.4.47

require (
	github.com/klauspost/compress v1.15.9 // indirect
	github.com/pierrec/lz4/v4 v4.1.15 // indirect
)

replace github.com/nrg-champ/circuitbreaker => ../../circuit_breaker
