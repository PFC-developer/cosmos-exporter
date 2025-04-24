// original here - https://gist.github.com/jumanzii/031cfea1b2aa3c2a43b63aa62a919285
package main

import (
	//"context"
	//oracletypes "github.com/Team-Kujira/core/x/oracle/types"
	"net/http"
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
type InitiaMetrics struct {
}

func NewInitiaMetrics(reg prometheus.Registerer, config *exporter.ServiceConfig) *InitiaMetrics {
	m := &InitiaMetrics{
		/*
			votePenaltyCount: prometheus.NewCounterVec(
				prometheus.CounterOpts{
					Name:        "cosmos_initia_oracle_vote_fail_count",
					Help:        "Vote fail count",
					ConstLabels: config.ConstLabels,
				},
				[]string{"type", "validator"},
			),

		*/
	}

	//reg.MustRegister(m.votePenaltyCount)

	return m
}

func getInitiaMetrics(wg *sync.WaitGroup, sublogger *zerolog.Logger, metrics *InitiaMetrics, s *exporter.Service, _ *exporter.ServiceConfig, validatorAddress sdk.ValAddress) {
	/*
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

	*/
}

func InitiaMetricHandler(w http.ResponseWriter, r *http.Request, s *exporter.Service) {
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
	intiaMetrics := NewInitiaMetrics(registry, s.Config)

	var wg sync.WaitGroup
	getInitiaMetrics(&wg, &sublogger, intiaMetrics, s, s.Config, myAddress)

	wg.Wait()

	h := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
	h.ServeHTTP(w, r)
	sublogger.Info().
		Str("method", "GET").
		Str("endpoint", "/metrics/initia").
		Float64("request-time", time.Since(requestStart).Seconds()).
		Msg("Request processed")
}
