package clock

import "time"

type Clock interface {
	Now() time.Time
}

type System struct{}

func (System) Now() time.Time {
	return time.Now().UTC()
}

type Fixed struct {
	T time.Time
}

func (f *Fixed) Now() time.Time {
	return f.T.UTC()
}

func (f *Fixed) Advance(d time.Duration) {
	f.T = f.T.Add(d)
}
