package exporter

import (
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

func (s *Service) SingleHandler(w http.ResponseWriter, r *http.Request) {
	requestStart := time.Now()

	sublogger := s.Log.With().
		Str("request-id", uuid.New().String()).
		Logger()

	registry := prometheus.NewRegistry()
	generalMetrics := NewGeneralMetrics(registry, s.Config)
	var validatorMetrics *ValidatorMetrics
	var paramsMetrics *ParamsMetrics
	var upgradeMetrics *UpgradeMetrics
	var walletMetrics *WalletMetrics

	var proposalMetrics *ProposalsMetrics
	var validatorVotingMetrics *ValidatorVotingMetrics

	if len(s.Validators) > 0 {
		validatorMetrics = NewValidatorMetrics(registry, s.Config)
	}
	if len(s.Wallets) > 0 {
		walletMetrics = NewWalletMetrics(registry, s.Config)
	}
	if s.Params {
		paramsMetrics = NewParamsMetrics(registry, s.Config)
	}
	if s.Upgrades {
		upgradeMetrics = NewUpgradeMetrics(registry, s.Config)
	}

	if s.Proposals {
		proposalMetrics = NewProposalsMetrics(registry, s.Config)
	}

	if s.Config.Votes && len(s.Validators) > 0 {
		validatorVotingMetrics = NewValidatorVotingMetrics(registry, s.Config)
	}

	var wg sync.WaitGroup

	GetGeneralMetrics(&wg, &sublogger, generalMetrics, s, s.Config)
	if paramsMetrics != nil {
		GetParamsMetrics(&wg, &sublogger, paramsMetrics, s, s.Config)
	}
	if upgradeMetrics != nil {
		DoUpgradeMetrics(&wg, &sublogger, upgradeMetrics, s, s.Config)
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
				go func(validator string) {
					defer val_wg.Done()
					sublogger.Debug().Str("address", validator).Msg("Fetching validator details")

					GetValidatorBasicMetrics(&wg, &sublogger, validatorMetrics, s, s.Config, valAddress)
				}(validator)

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
						Msg("Could not get active proposals V1 (general)")
				}
			} else {
				activeProps, err = s.GetActiveProposals(&sublogger)
				if err != nil {
					sublogger.Error().
						Err(err).
						Msg("Could not get active proposals (general)")
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
					GetProposalsVoteMetrics(&wg, &sublogger, validatorVotingMetrics, s, s.Config, propId, valAddress, accAddress)
					/*
						sublogger.Debug().
							Str("Validator", valAddress.String()).
							Str("Wallet", accAddress.String()).
							Uint64("Prop", propId).Msg("Get Vote")*/
				}
			}
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
				GetWalletMetrics(&wg, &sublogger, walletMetrics, s, s.Config, accAddress, false)
			}
		}
	}
	if s.Proposals {
		GetProposalsMetrics(&wg, &sublogger, proposalMetrics, s, s.Config, true)
	}
	wg.Wait()

	h := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
	h.ServeHTTP(w, r)
	sublogger.Info().
		Str("method", "GET").
		Str("endpoint", "/metrics").
		Str("type", "regular").
		Float64("request-time", time.Since(requestStart).Seconds()).
		Msg("Request processed")
}
