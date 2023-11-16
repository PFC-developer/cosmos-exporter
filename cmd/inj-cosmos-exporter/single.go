package main

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"main/pkg/exporter"
	"net/http"
	"sync"
	"time"
)

func InjSingleHandler(w http.ResponseWriter, r *http.Request, s *exporter.Service) {
	requestStart := time.Now()

	sublogger := log.With().
		Str("request-id", uuid.New().String()).
		Logger()

	registry := prometheus.NewRegistry()
	generalMetrics := exporter.NewGeneralMetrics(registry, s.Config)
	var validatorMetrics *exporter.ValidatorMetrics
	var paramsMetrics *exporter.ParamsMetrics
	var upgradeMetrics *exporter.UpgradeMetrics
	var walletMetrics *exporter.WalletMetrics
	var injMetrics *InjMetrics

	var proposalMetrics *exporter.ProposalsMetrics
	var validatorVotingMetrics *exporter.ValidatorVotingMetrics

	if len(s.Validators) > 0 {
		validatorMetrics = exporter.NewValidatorMetrics(registry, s.Config)
	}
	if len(s.Wallets) > 0 {
		walletMetrics = exporter.NewWalletMetrics(registry, s.Config)
	}
	if s.Params {
		paramsMetrics = exporter.NewParamsMetrics(registry, s.Config)
	}
	if s.Upgrades {
		upgradeMetrics = exporter.NewUpgradeMetrics(registry, s.Config)
	}

	if s.Proposals {
		proposalMetrics = exporter.NewProposalsMetrics(registry, s.Config)
	}
	if Peggo && Orchestrator != "" {
		injMetrics = NewInjMetrics(registry, s.Config)
	}
	if s.Config.Votes && len(s.Validators) > 0 {
		validatorVotingMetrics = exporter.NewValidatorVotingMetrics(registry, s.Config)
	}
	var wg sync.WaitGroup

	exporter.GetGeneralMetrics(&wg, &sublogger, generalMetrics, s, s.Config)
	if paramsMetrics != nil {
		exporter.GetParamsMetrics(&wg, &sublogger, paramsMetrics, s, s.Config)
	}
	if upgradeMetrics != nil {
		exporter.GetUpgradeMetrics(&wg, &sublogger, upgradeMetrics, s, s.Config)
	}
	if Orchestrator != "" && Peggo {
		accAddress, err := sdk.AccAddressFromBech32(Orchestrator)
		if err != nil {
			sublogger.Error().
				Str("address", Orchestrator).
				Err(err).
				Msg("doesn't appear valid")
		} else {
			getInjMetrics(&wg, &sublogger, injMetrics, s, s.Config, accAddress)
		}
	}
	if len(s.Wallets) > 0 {
		for _, wallet := range s.Wallets {
			accAddress, err := sdk.AccAddressFromBech32(wallet)
			if err != nil {
				sublogger.Error().
					Str("address", wallet).
					Err(err).
					Msg("Could not get wallet address")
			} else {
				exporter.GetWalletMetrics(&wg, &sublogger, walletMetrics, s, s.Config, accAddress, false)
			}
		}
	}
	if s.Proposals {
		exporter.GetProposalsMetrics(&wg, &sublogger, proposalMetrics, s, s.Config, true)
	}
	if len(s.Validators) > 0 {
		// use 2 groups.
		// the first group "val_wg" allows us to batch the initial validator call to get the moniker
		// the 'BasicMetrics' will then add a request to the outer wait 'wg'.
		// we ensure that all the requests are added by waiting for the 'val_wg' to finish before waiting on the 'wg'
		var val_wg sync.WaitGroup
		for _, validator := range s.Validators {
			valAddress, err := sdk.ValAddressFromBech32(validator)

			if err != nil {
				sublogger.Error().
					Str("address", validator).
					Err(err).
					Msg("Could not get validator address")

			} else {
				val_wg.Add(1)
				go func() {
					defer val_wg.Done()
					sublogger.Debug().Str("address", validator).Msg("Fetching validator details")

					exporter.GetValidatorBasicMetrics(&wg, &sublogger, validatorMetrics, s, s.Config, valAddress)
				}()

			}
		}
		val_wg.Wait()
	}
	if s.Config.Votes && len(s.Validators) > 0 {
		// use 2 groups.
		// the first group "prop_wg" allows us to batch the call to get the active props
		// the 'BasicMetrics' will then add a request to the outer wait 'wg'.
		// we ensure that all the requests are added by waiting for the 'val_wg' to finish before waiting on the 'wg'
		var prop_wg sync.WaitGroup
		prop_wg.Add(1)
		var activeProps []uint64

		go func() {
			defer prop_wg.Done()
			var err error
			if s.Config.PropV1 {
				activeProps, err = s.GetActiveProposalsV1(&sublogger)
				if err != nil {
					sublogger.Error().
						Err(err).
						Msg("Could not get active proposals V1")
				}
			} else {
				activeProps, err = s.GetActiveProposals(&sublogger)
				if err != nil {
					sublogger.Error().
						Err(err).
						Msg("Could not get active proposals")
				}
			}
		}()

		prop_wg.Wait()

		for _, validator := range s.Validators {
			valAddress, err := sdk.ValAddressFromBech32(validator)
			if err != nil {
				sublogger.Error().
					Str("address", validator).
					Err(err).
					Msg("Could not get validator address")

			} else {
				var accAddress sdk.AccAddress
				err := accAddress.Unmarshal(valAddress.Bytes())
				if err != nil {
					sublogger.Error().
						Str("address", validator).
						Err(err).
						Msg("Could not get acc address")

				}
				for _, propId := range activeProps {
					exporter.GetProposalsVoteMetrics(&wg, &sublogger, validatorVotingMetrics, s, s.Config, propId, valAddress, accAddress)
				}
			}
		}
	}
	wg.Wait()

	h := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
	h.ServeHTTP(w, r)
	sublogger.Info().
		Str("method", "GET").
		Str("endpoint", "/metrics").
		Str("type", "injective").
		Float64("request-time", time.Since(requestStart).Seconds()).
		Msg("Request processed")
}
