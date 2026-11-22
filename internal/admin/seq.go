package admin

import "time"

// Secret is the remote sequence that opens the panel. Heart does nothing on
// its own and Up/Down do nothing outside the panel, so nobody types this by
// accident; Mic is avoided because the remote sends Mic noise when it
// reconnects.
var Secret = []Key{KHeart, KUp, KHeart, KDown}

// SecretWindow is how long the whole sequence may take.
const SecretWindow = 5 * time.Second

// seq detects Secret in a stream of keys.
type seq struct {
	keys  []Key
	times []time.Time
}

// push adds a key and reports whether the sequence just completed.
func (s *seq) push(k Key, now time.Time) bool {
	s.keys = append(s.keys, k)
	s.times = append(s.times, now)
	if n := len(s.keys); n > len(Secret) {
		s.keys, s.times = s.keys[n-len(Secret):], s.times[n-len(Secret):]
	}
	if len(s.keys) < len(Secret) || now.Sub(s.times[0]) > SecretWindow {
		return false
	}
	for i := range Secret {
		if s.keys[i] != Secret[i] {
			return false
		}
	}
	s.keys, s.times = nil, nil
	return true
}
