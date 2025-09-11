// v1
// internal/analyze/analyze.go
package analyze

type Flags struct {
	BelowTarget bool `json:"belowTarget"`
	AboveTarget bool `json:"aboveTarget"`
	InBand      bool `json:"inBand"`
}

type Assessment struct {
	RoomID  string  `json:"roomId"`
	TempC   float64 `json:"tempC"`
	TargetC float64 `json:"targetC"`
	ErrorC  float64 `json:"errorC"` // temp - target
	Flags   Flags   `json:"flags"`
}

func Evaluate(room string, tempC, targetC, deadband float64) Assessment 
func Evaluate(room string, tempC, targetC, deadband float64) Assessment {
	err := tempC - targetC
	flags := Flags{
		BelowTarget: err < -deadband,
		AboveTarget: err > deadband,
		InBand:      -deadband <= err && err <= deadband,
	}
	return Assessment{RoomID: room, TempC: tempC, TargetC: targetC, ErrorC: err, Flags: flags}
}
