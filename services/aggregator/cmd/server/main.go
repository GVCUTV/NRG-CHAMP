// v3
// file: cmd/server/main.go
package main

import "NRG-CHAMP/aggregator/internal"

func main() {
	if err := internal.StartCmd(); err != nil {
		panic(err)
	}
}
