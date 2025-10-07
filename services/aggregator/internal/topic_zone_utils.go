// Package internal v0
// file: internal/topic_zone_utils.go
package internal

// extractZoneFromTopic returns the trailing segment after the last dot.
// When topics follow the `prefix.zone` format, this yields the desired zone slug.
// If the input does not contain a dot, the original string is returned.
func extractZoneFromTopic(topic string) string {
	last := -1
	for i := len(topic) - 1; i >= 0; i-- {
		if topic[i] == '.' {
			last = i
			break
		}
	}
	if last == -1 || last == len(topic)-1 {
		return topic
	}
	return topic[last+1:]
}
