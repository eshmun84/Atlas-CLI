package memory

import "time"

// Clock returns the current time. Tests may override via Store options.
type Clock func() time.Time

func defaultClock() time.Time { return time.Now().UTC() }
