package exporter

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
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
	//var kujiOracleMetrics *KujiMetrics
	var proposalMetrics *ProposalsMetrics

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
	/*
		if s.Oracle {
			kujiOracleMetrics = NewKujiMetrics(registry, s.Config)
		}
	*/

	if s.Proposals {
		proposalMetrics = NewProposalsMetrics(registry, s.Config)
	}

	var wg sync.WaitGroup

	GetGeneralMetrics(&wg, &sublogger, generalMetrics, s, s.Config)
	if paramsMetrics != nil {
		GetParamsMetrics(&wg, &sublogger, paramsMetrics, s, s.Config)
	}
	if upgradeMetrics != nil {
		GetUpgradeMetrics(&wg, &sublogger, upgradeMetrics, s, s.Config)
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

					GetValidatorBasicMetrics(&wg, &sublogger, validatorMetrics, s, s.Config, valAddress)
				}()
				/*
					if s.Oracle {
						sublogger.Debug().Str("address", validator).Msg("Fetching Kujira details")

						getKujiMetrics(&wg, &sublogger, kujiOracleMetrics, s, s.Config, valAddress)
					}

				*/
			}
		}
		val_wg.Wait()
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
