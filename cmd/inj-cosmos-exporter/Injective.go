// original here - https://gist.github.com/jumanzii/031cfea1b2aa3c2a43b63aa62a919285
package main

import (
	"encoding/json"
	"fmt"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/rs/zerolog"
	"main/pkg/exporter"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

/*
	type voteMissCounter struct {
		MissCount string `json:"miss_count"`
	}
*/
type InjMetrics struct {
	lastObservedNonce *prometheus.CounterVec
	lastClaimedEvent  *prometheus.CounterVec
}

func NewInjMetrics(reg prometheus.Registerer, config *exporter.ServiceConfig) *InjMetrics {
	m := &InjMetrics{
		lastObservedNonce: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name:        "cosmos_injective_peggy_last_observed_nonce",
				Help:        "Last observed nonce",
				ConstLabels: config.ConstLabels,
			},
			[]string{"type"},
		),
		lastClaimedEvent: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name:        "cosmos_injective_peggy_last_claimed",
				Help:        "Last claimed ethereum event",
				ConstLabels: config.ConstLabels,
			},
			[]string{"type"},
		),
	}

	reg.MustRegister(m.lastObservedNonce)
	reg.MustRegister(m.lastClaimedEvent)

	return m
}

type ModuleStateResponse struct {
	State ModuleState `json:"state"`
}

type ModuleState struct {
	LastObservedNonce string `json:"last_observed_nonce"`
}
type LastClaimEventResponse struct {
	LastClaimEvent LastClaimEvent `json:"last_claim_event"`
}
type LastClaimEvent struct {
	EventNonce  string `json:"ethereum_event_nonce"`
	EventHeight string `json:"ethereum_event_height"`
}

func getInjMetrics(wg *sync.WaitGroup, sublogger *zerolog.Logger, metrics *InjMetrics, _ *exporter.Service, _ *exporter.ServiceConfig, orchestratorAddress sdk.AccAddress) {

	wg.Add(1)
	go func() {
		defer wg.Done()
		sublogger.Debug().Msg("Started querying LCD peggy module state")
		queryStart := time.Now()

		requestURL := fmt.Sprintf("%s/peggy/v1/module_state", LCD)
		response, err := http.Get(requestURL)
		if err != nil {
			sublogger.Error().
				Err(err).
				Msg("Could not get peggy module state")
			return
		}
		moduleStateResponse := &ModuleStateResponse{}
		err = json.NewDecoder(response.Body).Decode(moduleStateResponse)
		if err != nil {
			sublogger.Error().
				Err(err).
				Msg("Could not decode LCD peggy module state")
			return
		}
		sublogger.Debug().Str("response", moduleStateResponse.State.LastObservedNonce).Msg("Response")
		sublogger.Debug().
			Float64("request-time", time.Since(queryStart).Seconds()).
			Msg("Finished querying oracle feeder metrics")

		missCount, err := strconv.ParseFloat(moduleStateResponse.State.LastObservedNonce, 64)
		if err != nil {
			sublogger.Error().
				Err(err).
				Msg("Could not get convert module state to float")
			return
		}
		metrics.lastObservedNonce.WithLabelValues("nonce").Add(missCount)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		sublogger.Debug().Msg("Started querying LCD peggy oracle event for orchestrator")
		queryStart := time.Now()

		requestURL := fmt.Sprintf("%s/peggy/v1/oracle/event/%s", LCD, orchestratorAddress.String())
		response, err := http.Get(requestURL)
		if err != nil {
			sublogger.Error().
				Err(err).
				Msg("Could not get peggy oracle event for orchestrator")
			return
		}
		eventStateResponse := &LastClaimEventResponse{}
		err = json.NewDecoder(response.Body).Decode(eventStateResponse)
		if err != nil {
			sublogger.Error().
				Err(err).
				Msg("Could not decode peggy oracle event for orchestrator")
			return
		}
		sublogger.Debug().Str("response", eventStateResponse.LastClaimEvent.EventNonce).Msg("Response")
		sublogger.Debug().
			Float64("request-time", time.Since(queryStart).Seconds()).
			Msg("Finished querying oracle feeder metrics")

		missCount, err := strconv.ParseFloat(eventStateResponse.LastClaimEvent.EventNonce, 64)
		if err != nil {
			sublogger.Error().
				Err(err).
				Msg("Could not get convert EventNonce to float")
			return
		}
		eventHeight, err := strconv.ParseFloat(eventStateResponse.LastClaimEvent.EventHeight, 64)
		if err != nil {
			sublogger.Error().
				Err(err).
				Msg("Could not get convert eventHeight to float")
			return
		}
		metrics.lastClaimedEvent.WithLabelValues("nonce").Add(missCount)
		metrics.lastClaimedEvent.WithLabelValues("event_height").Add(eventHeight)
	}()
}
func InjMetricHandler(w http.ResponseWriter, r *http.Request, s *exporter.Service) {
	requestStart := time.Now()

	sublogger := s.Log.With().
		Str("request-id", uuid.New().String()).
		Logger()

	address := r.URL.Query().Get("address")
	myAddress, err := sdk.AccAddressFromBech32(address)
	if err != nil {
		sublogger.Error().
			Str("address", address).
			Err(err).
			Msg("Could not get address")
		return
	}
	registry := prometheus.NewRegistry()
	injMetrics := NewInjMetrics(registry, s.Config)

	var wg sync.WaitGroup

	getInjMetrics(&wg, &sublogger, injMetrics, s, s.Config, myAddress)

	wg.Wait()

	h := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
	h.ServeHTTP(w, r)
	sublogger.Info().
		Str("method", "GET").
		Str("endpoint", "/metrics/injective").
		Float64("request-time", time.Since(requestStart).Seconds()).
		Msg("Request processed")
}
