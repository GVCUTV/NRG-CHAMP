package internal

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

// handleIngest accepts:
// - application/json: either a single object or an array of objects
// - text/plain or application/x-ndjson: newline-delimited JSON
func (s *server) handleIngest(w http.ResponseWriter, r *http.Request) {
	ct := r.Header.Get("Content-Type")
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			s.log.Error("error closing request body", "err", err)
		}
	}(r.Body)

	added := 0
	var errs []string

	push := func(rw readingWire) {
		rd, err := rw.toReading()
		if err != nil {
			errs = append(errs, err.Error())
			return
		}
		s.buf.add(rd)
		added++
	}

	if strings.Contains(ct, "application/json") {
		dec := json.NewDecoder(r.Body)
		dec.UseNumber()
		tok, err := dec.Token()
		if err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		switch v := tok.(type) {
		case json.Delim:
			if v == '{' {
				var rw readingWire
				if err := dec.Decode(&rw); err != nil {
					s.writeError(w, http.StatusBadRequest, "invalid JSON object")
					return
				}
				push(rw)
			} else if v == '[' {
				for dec.More() {
					var rw readingWire
					if err := dec.Decode(&rw); err != nil {
						errs = append(errs, "invalid array element")
						break
					}
					push(rw)
				}
			} else {
				s.writeError(w, http.StatusBadRequest, "unexpected JSON start")
				return
			}
		default:
			s.writeError(w, http.StatusBadRequest, "unexpected JSON")
			return
		}
	} else {
		// NDJSON fallback
		sc := bufio.NewScanner(r.Body)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" {
				continue
			}
			var rw readingWire
			if err := json.Unmarshal([]byte(line), &rw); err != nil {
				errs = append(errs, "bad ndjson line")
				continue
			}
			push(rw)
		}
		if err := sc.Err(); err != nil {
			errs = append(errs, err.Error())
		}
	}

	status := http.StatusOK
	if added == 0 {
		status = http.StatusBadRequest
	}
	resp := map[string]any{
		"ingested": added,
	}
	if len(errs) > 0 {
		resp["errors"] = errs
	}
	s.writeJSON(w, status, resp)
}
