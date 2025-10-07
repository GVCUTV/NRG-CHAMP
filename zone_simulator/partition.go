// v0
// zone_simulator/partition.go

package main

import (
	"encoding/binary"

	"github.com/segmentio/kafka-go"
)

// uniquePartitionIDs returns the unique partition identifiers present in the
// provided slice. The order of the returned slice is undefined and callers
// should perform any required sorting before indexing.
func uniquePartitionIDs(parts []kafka.Partition) []int {
	uniq := make(map[int]struct{}, len(parts))
	for _, part := range parts {
		uniq[part.ID] = struct{}{}
	}

	ids := make([]int, 0, len(uniq))
	for id := range uniq {
		ids = append(ids, id)
	}

	return ids
}

// murmur2JavaCompat returns the Java-compatible Murmur2 hash for Kafka
// partitioning. The implementation matches the hashing strategy used by the
// kafka-go Hash balancer and the official Java client. The returned value is a
// positive 32-bit integer suitable for modulus partition calculations.
func murmur2JavaCompat(key []byte) uint32 {
	const (
		seed = 0x9747b28c
		m    = 0x5bd1e995
		r    = 24
	)

	h := uint32(seed ^ len(key))
	data := key

	for len(data) >= 4 {
		k := binary.LittleEndian.Uint32(data[:4])
		data = data[4:]

		k *= m
		k ^= k >> r
		k *= m

		h *= m
		h ^= k
	}

	switch len(data) {
	case 3:
		h ^= uint32(data[2]) << 16
		fallthrough
	case 2:
		h ^= uint32(data[1]) << 8
		fallthrough
	case 1:
		h ^= uint32(data[0])
		h *= m
	}

	h ^= h >> 13
	h *= m
	h ^= h >> 15

	return h & 0x7fffffff
}
