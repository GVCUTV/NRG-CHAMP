package internal

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

func (rw *readingWire) toReading() (Reading, error) {
	var r Reading
	var err error

	r.DeviceID, err = toString(rw.DeviceID)
	if err != nil || r.DeviceID == "" {
		return r, fmt.Errorf("invalid deviceId: %v", err)
	}
	r.ZoneID, err = toString(rw.ZoneID)
	if err != nil || r.ZoneID == "" {
		return r, fmt.Errorf("invalid zoneId: %v", err)
	}
	r.Timestamp, err = toTime(rw.Timestamp)
	if err != nil || r.Timestamp.IsZero() {
		return r, fmt.Errorf("invalid timestamp: %v", err)
	}
	r.Temperature, err = toFloat(rw.Temperature)
	if err != nil {
		return r, fmt.Errorf("invalid temperature: %v", err)
	}
	r.Humidity, err = toFloat(rw.Humidity)
	if err != nil {
		return r, fmt.Errorf("invalid humidity: %v", err)
	}
	return r, nil
}

// /latest?zoneId=Z returns the most recent reading for zone Z.
func (s *server) handleLatest(w http.ResponseWriter, r *http.Request) {
	zone := r.URL.Query().Get("zoneId")
	if strings.TrimSpace(zone) == "" {
		s.writeError(w, http.StatusBadRequest, "missing zoneId")
		return
	}
	reading, ok := s.buf.latest(zone)
	if !ok {
		s.writeError(w, http.StatusNotFound, "no data for zoneId")
		return
	}
	s.writeJSON(w, http.StatusOK, reading)
}

/* /series?zoneId=Z&from=RFC3339&to=RFC3339 returns all readings for zone Z in [from, to].
 * 'from' and 'to' are optional; default to [now-window, now], i.e., last [size of the window] minutes.
 *
 * RFC3339 time is in the format "YYYY-MM-DDTH24:MI:SS"
 */
func (s *server) handleSeries(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	zone := q.Get("zoneId")
	if strings.TrimSpace(zone) == "" {
		s.writeError(w, http.StatusBadRequest, "missing zoneId")
		return
	}
	var from time.Time
	var to time.Time
	var err error
	if fr := q.Get("from"); fr != "" {
		from, err = time.Parse(time.RFC3339, fr)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, "bad 'from' (RFC3339)")
			return
		}
	} else {
		from = time.Now().Add(-s.buf.window)
	}
	if tr := q.Get("to"); tr != "" {
		to, err = time.Parse(time.RFC3339, tr)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, "bad 'to' (RFC3339)")
			return
		}
	} else {
		to = time.Now()
	}
	series := s.buf.series(zone, from, to)
	s.writeJSON(w, http.StatusOK, series)
}
