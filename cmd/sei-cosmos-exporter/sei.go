// original here - https://gist.github.com/jumanzii/031cfea1b2aa3c2a43b63aa62a919285
package main

import (
	"context"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/rs/zerolog"
	"main/pkg/exporter"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	oracletypes "github.com/sei-protocol/sei-chain/x/oracle/types"
)

type votePenaltyCounter struct {
	MissCount    string `json:"miss_count"`
	AbstainCount string `json:"abstain_count"`
	SuccessCount string `json:"success_count"`
}
type SeiMetrics struct {
	votePenaltyCount *prometheus.CounterVec
}

func NewSeiMetrics(reg prometheus.Registerer, config *exporter.ServiceConfig) *SeiMetrics {
	m := &SeiMetrics{
		votePenaltyCount: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name:        "cosmos_oracle_vote_penalty_count",
				Help:        "Vote penalty miss count",
				ConstLabels: config.ConstLabels,
			},
			[]string{"type"},
		),
	}

	reg.MustRegister(m.votePenaltyCount)

	return m
}
func getSeiMetrics(wg *sync.WaitGroup, sublogger *zerolog.Logger, metrics *SeiMetrics, s *exporter.Service, _ *exporter.ServiceConfig, validatorAddress sdk.ValAddress) {
	wg.Add(1)

	go func() {
		defer wg.Done()
		sublogger.Debug().Msg("Started querying oracle feeder metrics")
		queryStart := time.Now()

		oracleClient := oracletypes.NewQueryClient(s.GrpcConn)
		response, err := oracleClient.VotePenaltyCounter(context.Background(), &oracletypes.QueryVotePenaltyCounterRequest{ValidatorAddr: validatorAddress.String()})

		if err != nil {
			sublogger.Error().
				Err(err).
				Msg("Could not get oracle feeder metrics")
			return
		}

		sublogger.Debug().
			Float64("request-time", time.Since(queryStart).Seconds()).
			Msg("Finished querying oracle feeder metrics")

		missCount := float64(response.VotePenaltyCounter.MissCount)
		abstainCount := float64(response.VotePenaltyCounter.AbstainCount)
		successCount := float64(response.VotePenaltyCounter.SuccessCount)

		metrics.votePenaltyCount.WithLabelValues("miss").Add(missCount)
		metrics.votePenaltyCount.WithLabelValues("abstain").Add(abstainCount)
		metrics.votePenaltyCount.WithLabelValues("success").Add(successCount)

	}()

}
func OracleMetricHandler(w http.ResponseWriter, r *http.Request, s *exporter.Service, _ *exporter.ServiceConfig) {
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
	seiMetrics := NewSeiMetrics(registry, s.Config)

	var wg sync.WaitGroup
	getSeiMetrics(&wg, &sublogger, seiMetrics, s, s.Config, myAddress)

	wg.Wait()

	h := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
	h.ServeHTTP(w, r)
	sublogger.Info().
		Str("method", "GET").
		Str("endpoint", "/metrics/sei").
		Float64("request-time", time.Since(requestStart).Seconds()).
		Msg("Request processed")
}
