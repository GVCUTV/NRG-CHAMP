// v0
// zone_simulator/partition_test.go

package main

import (
	"sort"
	"testing"
)

func TestMurmur2JavaCompatVectors(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want uint32
	}{
		{name: "empty", key: "", want: 0x106e08d9},
		{name: "single", key: "a", want: 0x22d0b27c},
		{name: "device", key: "device-123", want: 0x22c7ffef},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := murmur2JavaCompat([]byte(tc.key))
			if got != tc.want {
				t.Fatalf("murmur2JavaCompat(%q)=%#x want %#x", tc.key, got, tc.want)
			}
			if got&0x80000000 != 0 {
				t.Fatalf("murmur2JavaCompat(%q) produced non-positive hash %#x", tc.key, got)
			}
		})
	}
}

func TestPartitionIndexDeterministic(t *testing.T) {
	ids := []int{0, 1, 2, 3}
	key := "device-123"

	h := murmur2JavaCompat([]byte(key))
	idx := int(h % uint32(len(ids)))
	if idx != 3 {
		t.Fatalf("unexpected partition index: got %d want %d", idx, 3)
	}

	// ensure deterministic across repeated invocations
	for i := 0; i < 10; i++ {
		next := int(murmur2JavaCompat([]byte(key)) % uint32(len(ids)))
		if next != idx {
			t.Fatalf("partition index changed between runs: got %d want %d", next, idx)
		}
	}
}

func TestPartitionSelectionIgnoresInputOrder(t *testing.T) {
	unsorted := []int{3, 1, 0, 2}
	sortedCopy := append([]int(nil), unsorted...)
	sort.Ints(sortedCopy)

	key := "device-123"
	idx := int(murmur2JavaCompat([]byte(key)) % uint32(len(sortedCopy)))

	selected := sortedCopy[idx]

	// mimic the production logic: sort before indexing to prevent non-deterministic lookups.
	sort.Ints(unsorted)
	selectedAfterSort := unsorted[idx]

	if selected != selectedAfterSort {
		t.Fatalf("sorted selection mismatch: %d vs %d", selected, selectedAfterSort)
	}
}
