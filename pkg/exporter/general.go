package exporter

import (
	"context"
	"math/big"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"

	tmservice "github.com/cosmos/cosmos-sdk/client/grpc/cmtservice"
	query "github.com/cosmos/cosmos-sdk/types/query"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	distributiontypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	govtypeV1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types/v1beta1"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"github.com/solarlabsteam/cosmos-exporter/pkg/cosmosdirectory"
)

type GeneralMetrics struct {
	bondedTokensGauge        prometheus.Gauge
	notBondedTokensGauge     prometheus.Gauge
	communityPoolGauge       *prometheus.GaugeVec
	supplyTotalGauge         *prometheus.GaugeVec
	latestBlockHeight        prometheus.Gauge
	syncing                  prometheus.Gauge
	tokenPrice               prometheus.Gauge
	govVotingPeriodProposals prometheus.Gauge
	// GetNodeInfo
	applicationVersion *prometheus.GaugeVec
	defaultNodeInfo    *prometheus.GaugeVec
}

func NewGeneralMetrics(reg prometheus.Registerer, config *ServiceConfig) *GeneralMetrics {
	m := &GeneralMetrics{
		bondedTokensGauge: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name:        "cosmos_general_bonded_tokens",
				Help:        "Bonded tokens",
				ConstLabels: config.ConstLabels,
			},
		),
		notBondedTokensGauge: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name:        "cosmos_general_not_bonded_tokens",
				Help:        "Not bonded tokens",
				ConstLabels: config.ConstLabels,
			},
		),
		communityPoolGauge: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name:        "cosmos_general_community_pool",
				Help:        "Community pool",
				ConstLabels: config.ConstLabels,
			},
			[]string{"denom"},
		),
		supplyTotalGauge: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name:        "cosmos_general_supply_total",
				Help:        "Total supply",
				ConstLabels: config.ConstLabels,
			},
			[]string{"denom"},
		),
		latestBlockHeight: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name:        "cosmos_latest_block_height",
				Help:        "Latest block height",
				ConstLabels: config.ConstLabels,
			},
		),
		syncing: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name:        "cosmos_node_syncing",
				Help:        "Is Node Syncing",
				ConstLabels: config.ConstLabels,
			},
		),
		tokenPrice: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name:        "cosmos_token_price",
				Help:        "Cosmos token price",
				ConstLabels: config.ConstLabels,
			},
		),
		govVotingPeriodProposals: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name:        "cosmos_gov_voting_period_proposals",
				Help:        "Voting period proposals",
				ConstLabels: config.ConstLabels,
			},
		),
		// GetNodeInfo
		applicationVersion: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name:        "cosmos_node_application_version",
				Help:        "application version info of the chain",
				ConstLabels: config.ConstLabels,
			},
			[]string{"chain_name", "app_version", "git_commit", "go_version", "cosmos_sdk_version"},
		),
		defaultNodeInfo: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name:        "cosmos_node_default_node_info",
				Help:        "default node info of the chain",
				ConstLabels: config.ConstLabels,
			},
			[]string{"network", "version", "moniker"},
		),
	}
	reg.MustRegister(m.bondedTokensGauge)
	reg.MustRegister(m.notBondedTokensGauge)
	reg.MustRegister(m.communityPoolGauge)
	reg.MustRegister(m.supplyTotalGauge)

	// registry.MustRegister(generalInflationGauge)
	// registry.MustRegister(generalAnnualProvisions)

	reg.MustRegister(m.latestBlockHeight)
	reg.MustRegister(m.syncing)
	if config.TokenPrice {
		reg.MustRegister(m.tokenPrice)
	}
	reg.MustRegister(m.govVotingPeriodProposals)
	// nodeInfo
	reg.MustRegister(m.applicationVersion)
	reg.MustRegister(m.defaultNodeInfo)

	return m

	/*
		generalInflationGauge := prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name:        "cosmos_general_inflation",
				Help:        "Total supply",
				ConstLabels: ConstLabels,
			},
		)
	*/
	/*
		generalAnnualProvisions := prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name:        "cosmos_general_annual_provisions",
				Help:        "Annual provisions",
				ConstLabels: ConstLabels,
			},
			[]string{"denom"},
		)

	*/
}

func GetGeneralMetrics(wg *sync.WaitGroup, sublogger *zerolog.Logger, metrics *GeneralMetrics, s *Service, config *ServiceConfig) {
	if config.TokenPrice {
		wg.Add(1)
		go func() {
			defer wg.Done()
			chain, err := cosmosdirectory.GetChainByChainID(config.ChainID)
			if err != nil {
				sublogger.Error().Err(err).Msg("Could not get chain information")
				return
			}

			price := chain.GetPriceUSD()
			metrics.tokenPrice.Set(price)
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		sublogger.Debug().Msg("Started querying latest block height")

		queryStart := time.Now()

		latest, err := s.GetLatestBlock()
		if err != nil {
			sublogger.Error().Err(err).Msg("Could not get latest block height")
			return
		}

		sublogger.Debug().
			Float64("request-time", time.Since(queryStart).Seconds()).
			Msg("Finished querying block height")

		metrics.latestBlockHeight.Set(latest)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		sublogger.Debug().Msg("Started querying node syncing")

		queryStart := time.Now()

		serviceClient := tmservice.NewServiceClient(s.GrpcConn)

		response, err := serviceClient.GetSyncing(
			context.Background(),
			&tmservice.GetSyncingRequest{},
		)
		if err != nil {
			sublogger.Error().Err(err).Msg("Could not get node syncing")
			return
		}

		sublogger.Debug().
			Float64("request-time", time.Since(queryStart).Seconds()).
			Msg("Finished querying node syncing")

		if response.GetSyncing() {
			metrics.syncing.Set(float64(1))
		} else {
			metrics.syncing.Set(float64(0))
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		sublogger.Debug().Msg("Started querying staking pool")
		queryStart := time.Now()

		stakingClient := stakingtypes.NewQueryClient(s.GrpcConn)
		response, err := stakingClient.Pool(
			context.Background(),
			&stakingtypes.QueryPoolRequest{},
		)
		if err != nil {
			sublogger.Error().Err(err).Msg("Could not get staking pool")
			return
		}

		sublogger.Debug().
			Float64("request-time", time.Since(queryStart).Seconds()).
			Msg("Finished querying staking pool")

		bondedTokensBigInt := response.Pool.BondedTokens.BigInt()
		bondedTokens, _ := new(big.Float).SetInt(bondedTokensBigInt).Float64()

		notBondedTokensBigInt := response.Pool.NotBondedTokens.BigInt()
		notBondedTokens, _ := new(big.Float).SetInt(notBondedTokensBigInt).Float64()

		metrics.bondedTokensGauge.Set(bondedTokens)
		metrics.notBondedTokensGauge.Set(notBondedTokens)


		//generalNotBondedTokensGauge.Set(float64(response.Pool.NotBondedTokens.Int64()))
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		sublogger.Debug().Msg("Started querying distribution community pool")
		queryStart := time.Now()

		distributionClient := distributiontypes.NewQueryClient(s.GrpcConn)
		response, err := distributionClient.CommunityPool(
			context.Background(),
			&distributiontypes.QueryCommunityPoolRequest{},
		)
		if err != nil {
			sublogger.Error().Err(err).Msg("Could not get distribution community pool")
			return
		}

		sublogger.Debug().
			Float64("request-time", time.Since(queryStart).Seconds()).
			Msg("Finished querying distribution community pool")

		for _, coin := range response.Pool {
			if value, err := strconv.ParseFloat(coin.Amount.String(), 64); err != nil {
				sublogger.Error().
					Err(err).
					Msg("Could not get community pool coin")
			} else {
				metrics.communityPoolGauge.With(prometheus.Labels{
					"denom": config.Denom,
				}).Set(value / config.DenomCoefficient)
			}
		}
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		sublogger.Debug().Msg("Started querying NodeInfo")
		queryStart := time.Now()

		serviceClient := tmservice.NewServiceClient(s.GrpcConn)
		response, err := serviceClient.GetNodeInfo(
			context.Background(),
			&tmservice.GetNodeInfoRequest{},
		)
		if err != nil {
			sublogger.Error().Err(err).Msg("Could not get tmService NodeInfo")
			return
		}

		sublogger.Debug().
			Float64("request-time", time.Since(queryStart).Seconds()).
			Msg("Finished querying NodeInfo")
		application := response.GetApplicationVersion()
		metrics.applicationVersion.With(prometheus.Labels{
			"chain_name":         application.Name,
			"app_version":        application.Version,
			"git_commit":         application.GitCommit,
			"go_version":         application.GoVersion,
			"cosmos_sdk_version": application.CosmosSdkVersion,
		}).Set(float64(1))

		nodeinfo := response.GetDefaultNodeInfo()

		metrics.defaultNodeInfo.With(prometheus.Labels{
			"network": nodeinfo.Network,
			"version": nodeinfo.Version,
			"moniker": nodeinfo.Moniker,
		}).Set(float64(1))
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		sublogger.Debug().Msg("Started querying bank total supply")
		queryStart := time.Now()

		bankClient := banktypes.NewQueryClient(s.GrpcConn)
		response, err := bankClient.TotalSupply(
			context.Background(),
			&banktypes.QueryTotalSupplyRequest{},
		)
		for {
			if err != nil {
				sublogger.Error().Err(err).Msg("Could not get bank total supply")
				return
			}

			sublogger.Debug().
				Float64("request-time", time.Since(queryStart).Seconds()).
				Msg("Finished querying bank total supply")

			for _, coin := range response.Supply {
				if value, err := strconv.ParseFloat(coin.Amount.String(), 64); err != nil {
					sublogger.Error().
						Err(err).
						Msg("Could not get total supply")
				} else {
					metrics.supplyTotalGauge.With(prometheus.Labels{
						"denom": coin.GetDenom(),
					}).Set(value)
				}
			}
			if response.Pagination.NextKey == nil {
				break
			}
			response, err = bankClient.TotalSupply(
				context.Background(),
				&banktypes.QueryTotalSupplyRequest{
					Pagination: &query.PageRequest{
						Key: response.Pagination.NextKey,
					},
				},
			)
		}
	}()
	/*
		wg.Add(1)
		go func() {
			defer wg.Done()
			sublogger.Debug().Msg("Started querying inflation")
			queryStart := time.Now()

				mintClient := minttypes.NewQueryClient(s.grpcConn)
				response, err := mintClient.Inflation(
					context.Background(),
					&minttypes.QueryInflationRequest{},
				)
				if err != nil {
					sublogger.Error().Err(err).Msg("Could not get inflation")
					return
				}

				sublogger.Debug().
					Float64("request-time", time.Since(queryStart).Seconds()).
					Msg("Finished querying inflation")

				if value, err := strconv.ParseFloat(response.Inflation.String(), 64); err != nil {
					sublogger.Error().
						Err(err).
						Msg("Could not get inflation")
				} else {
					generalInflationGauge.Set(value)
				}
			}()
	*/
	/*
		wg.Add(1)
		go func() {
			defer wg.Done()
			sublogger.Debug().Msg("Started querying annual provisions")
			queryStart := time.Now()

			mintClient := minttypes.NewQueryClient(s.grpcConn)
			response, err := mintClient.AnnualProvisions(
				context.Background(),
				&minttypes.QueryAnnualProvisionsRequest{},
			)
			if err != nil {
				sublogger.Error().Err(err).Msg("Could not get annual provisions")
				return
			}

			sublogger.Debug().
				Float64("request-time", time.Since(queryStart).Seconds()).
				Msg("Finished querying annual provisions")

			if value, err := strconv.ParseFloat(response.AnnualProvisions.String(), 64); err != nil {
				sublogger.Error().
					Err(err).
					Msg("Could not get annual provisions")
			} else {
				generalAnnualProvisions.With(prometheus.Labels{
					"denom": Denom,
				}).Set(value / DenomCoefficient)
			}
		}()
	*/

	if config.PropV1 {
		wg.Add(1)
		go func() {
			defer wg.Done()

			sublogger.Debug().Msg("Started querying global gov V1 params")

			govClient := govtypeV1.NewQueryClient(s.GrpcConn)
			proposals, err := govClient.Proposals(context.Background(), &govtypeV1.QueryProposalsRequest{
				ProposalStatus: govtypeV1.StatusVotingPeriod,
			})
			if err != nil {
				sublogger.Error().
					Err(err).
					Msg("Could not get active proposals v1 (general)")
			}
			proposalsCount := len(proposals.GetProposals())
			metrics.govVotingPeriodProposals.Set(float64(proposalsCount))
		}()
	} else {
		wg.Add(1)
		go func() {
			defer wg.Done()

			sublogger.Debug().Msg("Started querying global gov v1beta1 params")

			govClient := govtypes.NewQueryClient(s.GrpcConn)
			proposals, err := govClient.Proposals(context.Background(), &govtypes.QueryProposalsRequest{
				ProposalStatus: govtypes.StatusVotingPeriod,
			})
			if err != nil {
				sublogger.Error().
					Err(err).
					Msg("Could not get active proposals (v1beta1)")
			}

			proposalsCount := len(proposals.GetProposals())
			metrics.govVotingPeriodProposals.Set(float64(proposalsCount))
		}()
	}
}

func (s *Service) GeneralHandler(w http.ResponseWriter, r *http.Request) {
	requestStart := time.Now()

	sublogger := s.Log.With().
		Str("request-id", uuid.New().String()).
		Logger()

	registry := prometheus.NewRegistry()
	generalMetrics := NewGeneralMetrics(registry, s.Config)

	var wg sync.WaitGroup

	GetGeneralMetrics(&wg, &sublogger, generalMetrics, s, s.Config)

	wg.Wait()

	h := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
	h.ServeHTTP(w, r)
	sublogger.Info().
		Str("method", "GET").
		Str("endpoint", "/metrics/general").
		Float64("request-time", time.Since(requestStart).Seconds()).
		Msg("Request processed")
}
