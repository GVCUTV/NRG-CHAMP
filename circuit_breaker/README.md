<!-- v1 -->
<!-- README.md -->
# Circuit Breaker Module (Go)

- Directory/package: **circuitbreaker** (niente underscore)
- Module path: `github.com/nrg-champ/circuitbreaker`
- Solo standard library (`log/slog` per i log).

## Uso rapido
```go
cfg, _ := circuitbreaker.LoadConfigFromProperties("cb.properties")
hc,  _ := circuitbreaker.NewHTTPClient("users-api", cfg, "http://users/health", nil)
// ... req := http.NewRequest(...)
// resp, err := hc.Do(req)
```
Per Kafka, implementa `Producer` e fornisci un `probe(ctx)` che verifichi la reachability del broker.
