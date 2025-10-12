// v1
// go.mod
module github.com/your-org/assessment

go 1.22

require github.com/prometheus/client_golang v1.18.0

replace github.com/prometheus/client_golang => ./third_party/github.com/prometheus/client_golang
