// Copyright (c) 2020 Richard Youngkin. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package internal

import (
	"encoding/json"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// EndpointSummary is used to report an overview of the results of
// a load test run for a given endpoint.
type EndpointSummary struct {
	URL string
	// Method is the HTTP Method (e.g., GET, PUT, POST, DELETE)
	Method string
	// totalRequestDuration is the sum of all request run durations in seconds
	totalRequestDuration time.Duration
	// URL is the endpoint URL
	// HTTPStatusDist is a map of HTTP Status (e.g., 200, 201, 404, etc)
	// to the number of occurrences (value) for a given status (key)
	HTTPStatusDist map[int]int
	// TotalRqsts is the overall number of requests made during the run
	TotalRqsts int64
	// TotalRequestDuration is the sum of all request run durations in seconds
	TotalRequestDuration string
	// MaxRqstDuration is the longest request duration in microseconds
	maxRqstDuration time.Duration
	MaxRqstDuration string
	// MinRqstDuration is the smallest request duration in microseconds
	minRqstDuration time.Duration
	MinRqstDuration string
	// AvgRqstDuration is the average duration of a request in microseconds
	AvgRqstDuration string
}

// RunSummary is used to report an overview of the results of a
// load test run
type RunSummary struct {
	// RqstRatePerSec is the overall request rate per second
	// rounded to the nearest integer
	RqstRatePerSec float64
	// RunDuration is the wall clock time of the test in seconds
	RunDuration string
	// ResponseDistribution is distribution of response times. There will be
	// 11 bucket; 10 microseconds or less, between 10us and 100us,
	// 100us and 1ms, 1ms to 10ms, 10ms to 100ms, 100ms to 1s, 1s to 1.1s,
	// 1.1s to 1.5s, 1.5s to 1.8s, 1.8s to 2.5s, 2.5s and above
	//ResponseDistribution map[float32]int
	// HTTPStatusDistribution is the distribution of HTTP response statuses
	//HTTPStatusDistribution map[string]int
	// MaxRqstRatePerSec is the maximum request rate per second
	// over 1/10th of the run duration or number of requests
	//MaxRqstRatePerSec int
	// MinRqstRatePerSec is the maximum request rate per second
	// over 1/10th of the run duration or number of requests
	//MinRqstRatePerSec int
	// TotalRqsts is the overall number of requests made during the run
	TotalRqsts int64
	// TotalRequestDuration is the sum of all request run durations in seconds
	TotalRequestDuration string
	// MaxRqstDuration is the longest request duration in microseconds
	maxRqstDuration time.Duration
	MaxRqstDuration string
	// MinRqstDuration is the smallest request duration in microseconds
	minRqstDuration time.Duration
	MinRqstDuration string
	// AvgRqstDuration is the average duration of a request in microseconds
	AvgRqstDuration string
	// EndpointOverviewSummary describes how often each endpoint was called.
	// It is a map keyed by URL of a map keyed by HTTP verb with a value of
	// number of requests. So it's a summary of how often each HTTP verb
	// was called on each endpoint.
	EndpointOverviewSummary map[string]map[string]int
	// EndpointRunSummary is the per endpoint summary of results keyed by URL
	EndpointRunSummary map[string]*EndpointSummary
}

// ResponseHandler is responsible for accepting, summarizing, and reporting
// on the overall load test results.
type ResponseHandler struct {
	ResponseC chan Response
	DoneC     chan struct{}
}

// Start begins the process of accepting responses. It expects to be run as a goroutine
func (rh ResponseHandler) Start() {
	log.Debug().Msg("ResponseHandler starting")
	epRunSummary := make(map[string]*EndpointSummary)
	runSummary := RunSummary{maxRqstDuration: -1, minRqstDuration: time.Duration(math.MaxInt64)}
	runSummary.EndpointOverviewSummary = make(map[string]map[string]int)

	var totalDurationSummary time.Duration
	var once sync.Once
	var start time.Time

	finishFunc := func() {
		runTime := time.Since(start)
		runSummary.RunDuration = runTime.String()
		runSummary.TotalRequestDuration = totalDurationSummary.String()
		runSummary.MaxRqstDuration = runSummary.maxRqstDuration.String()
		runSummary.MinRqstDuration = runSummary.minRqstDuration.String()
		avgRqstDuration := time.Duration(0)
		if runSummary.TotalRqsts > 0 {
			avgRqstDuration = totalDurationSummary / time.Duration(runSummary.TotalRqsts)
		}
		runSummary.AvgRqstDuration = avgRqstDuration.String()

		// run times shorter than 1 second will result in a 'RqstRatePerSec' being zero due to rounding
		runDurInMillis := runTime / time.Millisecond
		if runDurInMillis > 0 {
			runSummary.RqstRatePerSec = (float64(runSummary.TotalRqsts) / float64(runTime)) * float64(time.Second)
		}
		log.Debug().Msgf("NumRqsts: %d, RunDur in millis: %d, Rqsts/millis: %f, TotalRqsts/RunDur: %f", runSummary.TotalRqsts, int64(runDurInMillis), runSummary.RqstRatePerSec, float64(runSummary.TotalRqsts)/float64(runDurInMillis))
		runSummary.EndpointRunSummary = epRunSummary

		for _, epSumm := range epRunSummary {
			epSumm.MaxRqstDuration = epSumm.maxRqstDuration.String()
			epSumm.MinRqstDuration = epSumm.minRqstDuration.String()
			epSumm.AvgRqstDuration = "0s"
			if epSumm.TotalRqsts > 0 {
				epSumm.AvgRqstDuration = (epSumm.totalRequestDuration / time.Duration(epSumm.TotalRqsts)).String()
			}
			epSumm.TotalRequestDuration = epSumm.totalRequestDuration.String()
			log.Debug().Msgf("EndpointSummary: %+v", epSumm)
		}

		rsjson, err := json.Marshal(runSummary)
		if err != nil {
			log.Error().Err(err).Msgf("error marshaling RunSummary into string: %+v.\n", runSummary)
			return
		}

		fmt.Printf("%s\n", string(rsjson))
		close(rh.DoneC)
	}

	for {
		select {
		case resp, ok := <-rh.ResponseC:
			once.Do(func() { start = time.Now() })
			if !ok {
				log.Info().Msg("ResponseHandler: Summarizing results and exiting")
				finishFunc()
				return
			}

			runSummary.TotalRqsts++
			totalDurationSummary = totalDurationSummary + resp.RequestDuration
			if resp.RequestDuration > runSummary.maxRqstDuration {
				runSummary.maxRqstDuration = resp.RequestDuration
			}
			if resp.RequestDuration < runSummary.minRqstDuration {
				runSummary.minRqstDuration = resp.RequestDuration
			}

			var eqRqstCount map[string]int
			eqRqstCount, found := runSummary.EndpointOverviewSummary[resp.Endpoint.URL]
			if !found {
				runSummary.EndpointOverviewSummary[resp.Endpoint.URL] = make(map[string]int)
				eqRqstCount = runSummary.EndpointOverviewSummary[resp.Endpoint.URL]
			}
			eqRqstCount[resp.Endpoint.Method]++

			var epSumm *EndpointSummary
			epSumm, found = epRunSummary[resp.Endpoint.URL]
			if !found {
				epSumm = &EndpointSummary{
					URL:             resp.Endpoint.URL,
					Method:          resp.Endpoint.Method,
					HTTPStatusDist:  make(map[int]int),
					maxRqstDuration: -1,
					minRqstDuration: time.Duration(math.MaxInt64),
				}
				epRunSummary[resp.Endpoint.URL] = epSumm
			}

			epSumm.TotalRqsts++
			epSumm.totalRequestDuration = epSumm.totalRequestDuration + resp.RequestDuration

			if resp.RequestDuration > epSumm.maxRqstDuration {
				epSumm.maxRqstDuration = resp.RequestDuration
			}
			if resp.RequestDuration < epSumm.minRqstDuration {
				epSumm.minRqstDuration = resp.RequestDuration
			}

			_, ok = epSumm.HTTPStatusDist[resp.HTTPStatus]
			if !ok {
				epSumm.HTTPStatusDist[resp.HTTPStatus] = 0
			}
			epSumm.HTTPStatusDist[resp.HTTPStatus]++
		}
	}
}
