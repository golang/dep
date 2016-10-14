package gps

import "time"

type metrics struct {
	stack []string
	times map[string]time.Duration
	last  time.Time
}

func newMetrics() *metrics {
	return &metrics{
		stack: []string{"other"},
		times: map[string]time.Duration{
			"other": 0,
		},
		last: time.Now(),
	}
}

func (m *metrics) push(name string) {
	cn := m.stack[len(m.stack)-1]
	times[cn] = times[cn] + time.Since(m.last)

	m.stack = append(m.stack, name)
	m.last = time.Now()
}

func (m *metrics) pop() {
	on = m.stack[len(m.stack)-1]
	times[on] = times[on] + time.Since(m.last)

	m.stack = m.stack[:len(m.stack)-1]
	m.last = time.Now()
}
