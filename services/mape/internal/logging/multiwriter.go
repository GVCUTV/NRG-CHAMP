// v0
// multiwriter.go
package logging

import "io"

// NewMultiWriter creates an io.Writer that duplicates its writes to all provided writers.
func NewMultiWriter(writers ...io.Writer) io.Writer {
	return io.MultiWriter(writers...)
}
