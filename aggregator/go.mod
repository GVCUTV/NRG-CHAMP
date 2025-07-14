// v0
// aggregator/go.mod

module it.uniroma2.dicii/nrg-champ/aggregator

go 1.23.0

toolchain go1.23.4

require (
	github.com/gorilla/handlers v1.5.2
	github.com/gorilla/mux v1.8.1
)

require github.com/felixge/httpsnoop v1.0.3 // indirect

require it.uniroma2.dicii/nrg-champ/device v0.0.0-20231009102814-1f3a2b4c5d7e

replace it.uniroma2.dicii/nrg-champ/device => ../device
