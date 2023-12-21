// original here - https://gist.github.com/jumanzii/031cfea1b2aa3c2a43b63aa62a919285
package main

import (
	"context"
	"net/http"
	"sync"
	"time"

	oracletypes "github.com/Team-Kujira/core/x/oracle/types"
	"github.com/google/uuid"
	"github.com/pfc-developer/cosmos-exporter/pkg/exporter"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

/*
	type voteMissCounter struct {
		MissCount string `json:"miss_count"`
	}
*/
type KujiMetrics struct {
	votePenaltyCount *prometheus.CounterVec
}

func NewKujiMetrics(reg prometheus.Registerer, config *exporter.ServiceConfig) *KujiMetrics {
	m := &KujiMetrics{
		votePenaltyCount: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name:        "cosmos_kujira_oracle_vote_miss_count",
				Help:        "Vote miss count",
				ConstLabels: config.ConstLabels,
			},
			[]string{"type", "validator"},
		),
	}

	reg.MustRegister(m.votePenaltyCount)

	return m
}

func getKujiMetrics(wg *sync.WaitGroup, sublogger *zerolog.Logger, metrics *KujiMetrics, s *exporter.Service, _ *exporter.ServiceConfig, validatorAddress sdk.ValAddress) {
	wg.Add(1)

	go func() {
		defer wg.Done()
		sublogger.Debug().Msg("Started querying oracle feeder metrics")
		queryStart := time.Now()

		oracleClient := oracletypes.NewQueryClient(s.GrpcConn)
		response, err := oracleClient.MissCounter(context.Background(), &oracletypes.QueryMissCounterRequest{ValidatorAddr: validatorAddress.String()})
		if err != nil {
			sublogger.Error().
				Err(err).
				Msg("Could not get oracle feeder metrics")
			return
		}

		sublogger.Debug().
			Float64("request-time", time.Since(queryStart).Seconds()).
			Msg("Finished querying oracle feeder metrics")

		missCount := float64(response.MissCounter)

		metrics.votePenaltyCount.WithLabelValues("miss", validatorAddress.String()).Add(missCount)
	}()
}

func KujiraMetricHandler(w http.ResponseWriter, r *http.Request, s *exporter.Service) {
	requestStart := time.Now()

	sublogger := s.Log.With().
		Str("request-id", uuid.New().String()).
		Logger()

	address := r.URL.Query().Get("address")
	myAddress, err := sdk.ValAddressFromBech32(address)
	if err != nil {
		sublogger.Error().
			Str("address", address).
			Err(err).
			Msg("Could not get address")
		return
	}
	registry := prometheus.NewRegistry()
	kujiMetrics := NewKujiMetrics(registry, s.Config)

	var wg sync.WaitGroup
	getKujiMetrics(&wg, &sublogger, kujiMetrics, s, s.Config, myAddress)

	wg.Wait()

	h := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
	h.ServeHTTP(w, r)
	sublogger.Info().
		Str("method", "GET").
		Str("endpoint", "/metrics/kujira").
		Float64("request-time", time.Since(requestStart).Seconds()).
		Msg("Request processed")
}
