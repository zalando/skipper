// Package httptest is a test infrastructure package to support
// continuous stream of requests.
package httptest

import (
	"io"
	"strconv"
	"time"

	vegeta "github.com/tsenart/vegeta/lib"
)

type VegetaAttacker struct {
	attacker *vegeta.Attacker
	metrics  *vegeta.Metrics
	rate     *vegeta.Rate
	targeter vegeta.Targeter
}

func NewVegetaAttacker(url string, freq int, per time.Duration, timeout time.Duration) *VegetaAttacker {
	atk := vegeta.NewAttacker(
		vegeta.Connections(10),
		vegeta.H2C(false),
		vegeta.HTTP2(false),
		vegeta.KeepAlive(true),
		vegeta.MaxWorkers(10),
		vegeta.Redirects(0),
		vegeta.Timeout(timeout),
		vegeta.Workers(5),
	)

	tr := vegeta.NewStaticTargeter(vegeta.Target{Method: "GET", URL: url})
	rate := vegeta.Rate{Freq: freq, Per: per}

	m := vegeta.Metrics{
		Histogram: &vegeta.Histogram{
			Buckets: []time.Duration{
				0,
				10 * time.Microsecond,
				50 * time.Microsecond,
				100 * time.Microsecond,
				500 * time.Microsecond,
				1 * time.Millisecond,
				5 * time.Millisecond,
				10 * time.Millisecond,
				25 * time.Millisecond,
				50 * time.Millisecond,
				100 * time.Millisecond,
				1000 * time.Millisecond,
			},
		},
	}

	return &VegetaAttacker{
		attacker: atk,
		metrics:  &m,
		rate:     &rate,
		targeter: tr,
	}
}

func (atk *VegetaAttacker) Attack(w io.Writer, d time.Duration, name string) {
	for res := range atk.attacker.Attack(atk.targeter, atk.rate, d, name) {
		if res == nil {
			continue
		}
		atk.metrics.Add(res)
	}
	atk.metrics.Close()
	reporter := vegeta.NewTextReporter(atk.metrics)
	reporter.Report(w)
}

func (atk *VegetaAttacker) Metrics() *vegeta.Metrics {
	return atk.metrics
}

func (atk *VegetaAttacker) Success() float64 {
	return atk.metrics.Success
}

func (atk *VegetaAttacker) TotalRequests() uint64 {
	return atk.metrics.Requests
}

func (atk *VegetaAttacker) TotalSuccess() float64 {
	return atk.metrics.Success * float64(atk.metrics.Requests)
}

func (atk *VegetaAttacker) CountStatus(code int) (int, bool) {
	cnt, ok := atk.metrics.StatusCodes[strconv.Itoa(code)]
	return cnt, ok
}
