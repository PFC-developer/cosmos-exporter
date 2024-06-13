// original here - https://gist.github.com/jumanzii/031cfea1b2aa3c2a43b63aa62a919285
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/pfc-developer/cosmos-exporter/pkg/exporter"
)

/*
	type voteMissCounter struct {
		MissCount string `json:"miss_count"`
	}
*/
type PryzmMetrics struct {
	missCounter *prometheus.CounterVec
}

func NewPryzmMetrics(reg prometheus.Registerer, config *exporter.ServiceConfig) *PryzmMetrics {
	m := &PryzmMetrics{
		missCounter: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name:        "cosmos_pryzm_feeder_miss_counter",
				Help:        "miss counter",
				ConstLabels: config.ConstLabels,
			},
			[]string{"type"},
		),
	}

	reg.MustRegister(m.missCounter)

	return m
}

type MissCounterResponse struct {
	Miss MissCounterM `json:"miss_counter"`
}

type MissCounterM struct {
	Validator string `json:"validator"`
	Counter   string `json:"counter"`
}

func doPryzmMetrics(wg *sync.WaitGroup, sublogger *zerolog.Logger, metrics *PryzmMetrics, _ *exporter.Service, _ *exporter.ServiceConfig, validatorAddress sdk.ValAddress) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		sublogger.Debug().Msg("Started querying LCD peggy module state")
		queryStart := time.Now()

		requestURL := fmt.Sprintf("%s/refractedlabs/oracle/v1/miss_counter/%s", LCD, validatorAddress.String())
		response, err := http.Get(requestURL) // #nosec
		if err != nil {
			sublogger.Error().
				Err(err).
				Msg("Could not get peggy module state")
			return
		}
		moduleStateResponse := &MissCounterResponse{}
		err = json.NewDecoder(response.Body).Decode(moduleStateResponse)
		if err != nil {
			sublogger.Error().
				Err(err).
				Msg("Could not decode LCD peggy module state")
			return
		}
		sublogger.Debug().Str("response", moduleStateResponse.Miss.Counter).Msg("Response")
		sublogger.Debug().
			Float64("request-time", time.Since(queryStart).Seconds()).
			Msg("Finished querying oracle feeder metrics")

		missCount, err := strconv.ParseFloat(moduleStateResponse.Miss.Counter, 64)
		if err != nil {
			sublogger.Error().
				Err(err).
				Msg("Could not get convert module state to float")
			return
		}
		metrics.missCounter.WithLabelValues("miss").Add(missCount)
	}()
}

func PryzmMetricHandler(w http.ResponseWriter, r *http.Request, s *exporter.Service) {
	requestStart := time.Now()

	sublogger := s.Log.With().
		Str("request-id", uuid.New().String()).
		Logger()

	address := r.URL.Query().Get("validator")
	myAddress, err := sdk.ValAddressFromBech32(address)
	if err != nil {
		sublogger.Error().
			Str("address", address).
			Err(err).
			Msg("Could not get address")
		return
	}
	registry := prometheus.NewRegistry()
	pryzmMetrics := NewPryzmMetrics(registry, s.Config)

	var wg sync.WaitGroup

	doPryzmMetrics(&wg, &sublogger, pryzmMetrics, s, s.Config, myAddress)

	wg.Wait()

	h := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
	h.ServeHTTP(w, r)
	sublogger.Info().
		Str("method", "GET").
		Str("endpoint", "/metrics/pryzm").
		Float64("request-time", time.Since(requestStart).Seconds()).
		Msg("Request processed")
}
