package exporter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"

	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/query"
	govtypeV1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types/v1beta1"
)

type ProposalsMetrics struct {
	proposalsGauge *prometheus.GaugeVec
}
type ValidatorVotingMetrics struct {
	validatorVoting *prometheus.GaugeVec
}

type proposalMeta struct {
	Title string `json:"title"`
}

func NewProposalsMetrics(reg prometheus.Registerer, config *ServiceConfig) *ProposalsMetrics {
	m := &ProposalsMetrics{
		proposalsGauge: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name:        "cosmos_proposals",
				Help:        "Proposals of Cosmos-based blockchain",
				ConstLabels: config.ConstLabels,
			},
			[]string{"title", "status", "voting_start_time", "voting_end_time"},
		),
	}
	reg.MustRegister(m.proposalsGauge)
	return m
}

func NewValidatorVotingMetrics(reg prometheus.Registerer, config *ServiceConfig) *ValidatorVotingMetrics {
	m := &ValidatorVotingMetrics{
		validatorVoting: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name:        "cosmos_validator_voting_proposals",
				Help:        "Active Proposals of Cosmos-based blockchain, and how a validator voted",
				ConstLabels: config.ConstLabels,
			},
			[]string{"id", "validator", "voted", "vote_option"},
		),
	}
	reg.MustRegister(m.validatorVoting)
	return m
}

func GetProposalsMetrics(wg *sync.WaitGroup, sublogger *zerolog.Logger, metrics *ProposalsMetrics, s *Service, config *ServiceConfig, activeOnly bool) {
	if config.PropV1 {
		wg.Add(1)
		go func() {
			defer wg.Done()

			sublogger.Debug().Msg("Started querying v1 proposals")
			queryStart := time.Now()

			govClient := govtypeV1.NewQueryClient(s.GrpcConn)

			var propReq govtypeV1.QueryProposalsRequest
			if activeOnly {
				propReq = govtypeV1.QueryProposalsRequest{ProposalStatus: govtypeV1.StatusVotingPeriod, Pagination: &query.PageRequest{Reverse: true}}
			} else {
				propReq = govtypeV1.QueryProposalsRequest{Pagination: &query.PageRequest{Reverse: true}}
			}
			proposalsResponse, err := govClient.Proposals(
				context.Background(),
				&propReq,
			)
			if err != nil {
				sublogger.Error().Err(err).Msg("Could not get proposals (v1-props)")
				return
			}

			sublogger.Debug().
				Float64("request-time", time.Since(queryStart).Seconds()).
				Msg("Finished querying proposals")
			proposals := proposalsResponse.Proposals

			sublogger.Debug().
				Int("proposalsLength", len(proposals)).
				Msg("Proposals info")

			// cdc := codec.NewProtoCodec(cdcRegistry)
			for _, proposal := range proposals {
				var title string = ""
				if len(proposal.Metadata) > 0 {
					var metadata proposalMeta
					t := strings.Trim(proposal.Metadata, " ")
					if strings.HasPrefix(t, "{") {
						err := json.Unmarshal([]byte(proposal.Metadata), &metadata)
						if err != nil {
							sublogger.Error().
								Str("proposal_id", fmt.Sprint(proposal.Id)).
								Err(err).
								Msg("Could not parse proposal metadata field")
						} else {
							title = metadata.Title
						}
					} else if strings.HasPrefix(t, "ipfs://") {
						title = t
					} else {
						title = fmt.Sprintf("Proposal %d has unknown metadata", proposal.Id)
					}
				} else {
					sublogger.Info().
						Str("proposal_id", fmt.Sprint(proposal.Id)).
						Msg("Does not have metadata?")
					title = fmt.Sprintf("Proposal %d has no metadata", proposal.Id)
				}
				if proposal.VotingStartTime == nil || proposal.VotingEndTime == nil {
					metrics.proposalsGauge.With(prometheus.Labels{
						"title":             title,
						"status":            proposal.Status.String(),
						"voting_start_time": "nil",
						"voting_end_time":   "nil",
					}).Set(float64(proposal.Id))
				} else {
					metrics.proposalsGauge.With(prometheus.Labels{
						"title":             title,
						"status":            proposal.Status.String(),
						"voting_start_time": proposal.VotingStartTime.String(),
						"voting_end_time":   proposal.VotingEndTime.String(),
					}).Set(float64(proposal.Id))
				}
			}
		}()
	} else {
		wg.Add(1)
		go func() {
			defer wg.Done()

			var proposals []govtypes.Proposal

			sublogger.Debug().Msg("Started querying v1beta1 proposals")
			queryStart := time.Now()

			govClient := govtypes.NewQueryClient(s.GrpcConn)

			var propReq govtypes.QueryProposalsRequest
			if activeOnly {
				propReq = govtypes.QueryProposalsRequest{ProposalStatus: govtypes.StatusVotingPeriod, Pagination: &query.PageRequest{Reverse: true}}
			} else {
				propReq = govtypes.QueryProposalsRequest{Pagination: &query.PageRequest{Reverse: true}}
			}
			proposalsResponse, err := govClient.Proposals(
				context.Background(),
				&propReq,
			)
			if err != nil {
				sublogger.Error().Err(err).Msg("Could not get proposals (v1beta1-props)")
				return
			}

			sublogger.Debug().
				Float64("request-time", time.Since(queryStart).Seconds()).
				Msg("Finished querying proposals")
			proposals = proposalsResponse.Proposals

			sublogger.Debug().
				Int("proposalsLength", len(proposals)).
				Msg("Proposals info")

			cdcRegistry := codectypes.NewInterfaceRegistry()
			cdc := codec.NewProtoCodec(cdcRegistry)
			for _, proposal := range proposals {

				var content govtypes.TextProposal
				err := cdc.Unmarshal(proposal.Content.Value, &content)
				if err != nil {
					sublogger.Error().
						Str("proposal_id", fmt.Sprint(proposal.ProposalId)).
						Err(err).
						Msg("Could not parse proposal content")
				}

				metrics.proposalsGauge.With(prometheus.Labels{
					"title":             content.Title,
					"status":            proposal.Status.String(),
					"voting_start_time": proposal.VotingStartTime.String(),
					"voting_end_time":   proposal.VotingEndTime.String(),
				}).Set(float64(proposal.ProposalId))

			}
		}()
	}
}

func GetProposalsVoteMetrics(wg *sync.WaitGroup, sublogger *zerolog.Logger, metrics *ValidatorVotingMetrics, s *Service, _ *ServiceConfig, id uint64, validator types.ValAddress, wallet types.AccAddress) {
	wg.Add(1)
	go func() {
		defer wg.Done()

		var proposals []govtypes.Proposal

		sublogger.Debug().Msg("Started querying v1beta1 proposals")
		queryStart := time.Now()

		govClient := govtypes.NewQueryClient(s.GrpcConn)

		voteReq := govtypes.QueryVoteRequest{ProposalId: id, Voter: wallet.String()}

		voteResponse, err := govClient.Vote(
			context.Background(),
			&voteReq,
		)
		if err != nil {
			metrics.validatorVoting.With(prometheus.Labels{
				"id":          fmt.Sprintf("%d", id),
				"validator":   validator.String(),
				"voted":       "no",
				"vote_option": "NOT_VOTED",
			}).Set(float64(0))

			sublogger.Debug().Err(err).Msg("Could not get vote")
			return
		}

		sublogger.Debug().
			Float64("request-time", time.Since(queryStart).Seconds()).
			Msg("Finished getting vote")

		sublogger.Debug().
			Int("proposalsLength", len(proposals)).
			Msg("Proposals info")

		//	"id",  "validator", "vote", "vote_option"
		for _, voteOption := range voteResponse.Vote.Options {
			metrics.validatorVoting.With(prometheus.Labels{
				"id":          fmt.Sprintf("%d", id),
				"validator":   validator.String(),
				"voted":       "yes",
				"vote_option": voteOption.Option.String(),
			}).Set(float64(voteOption.Size()))
		}
	}()
}

func (s *Service) GetActiveProposalsV1(sublogger *zerolog.Logger) ([]uint64, error) {
	sublogger.Debug().Msg("Started querying v1 proposals")
	queryStart := time.Now()

	govClient := govtypeV1.NewQueryClient(s.GrpcConn)

	var propReq govtypeV1.QueryProposalsRequest

	propReq = govtypeV1.QueryProposalsRequest{ProposalStatus: govtypeV1.StatusVotingPeriod, Pagination: &query.PageRequest{Reverse: true}}

	proposalsResponse, err := govClient.Proposals(
		context.Background(),
		&propReq,
	)
	if err != nil {
		sublogger.Error().Err(err).Msg("Could not get proposals-activeV1")
		return nil, err
	}

	sublogger.Debug().
		Float64("request-time", time.Since(queryStart).Seconds()).
		Msg("Finished querying proposals")

	var proposals []uint64
	for _, prop := range proposalsResponse.Proposals {
		if prop.Status == govtypeV1.ProposalStatus_PROPOSAL_STATUS_VOTING_PERIOD {
			proposals = append(proposals, prop.Id)
		}
	}
	return proposals, nil
}

func (s *Service) GetActiveProposals(sublogger *zerolog.Logger) ([]uint64, error) {
	sublogger.Debug().Msg("Started querying v1 proposals")
	queryStart := time.Now()

	govClient := govtypes.NewQueryClient(s.GrpcConn)

	var propReq govtypes.QueryProposalsRequest

	propReq = govtypes.QueryProposalsRequest{ProposalStatus: govtypes.StatusVotingPeriod, Pagination: &query.PageRequest{Reverse: true}}

	proposalsResponse, err := govClient.Proposals(
		context.Background(),
		&propReq,
	)
	if err != nil {
		sublogger.Error().Err(err).Msg("Could not get proposals-active")
		return nil, err
	}

	sublogger.Debug().
		Float64("request-time", time.Since(queryStart).Seconds()).
		Msg("Finished querying proposals")
	var proposals []uint64
	for _, prop := range proposalsResponse.Proposals {
		if prop.Status == govtypes.StatusVotingPeriod {
			proposals = append(proposals, prop.ProposalId)
		}
	}
	return proposals, nil
}

func (s *Service) ProposalsHandler(w http.ResponseWriter, r *http.Request) {
	requestStart := time.Now()

	sublogger := s.Log.With().
		Str("request-id", uuid.New().String()).
		Logger()

	registry := prometheus.NewRegistry()
	proposalsMetrics := NewProposalsMetrics(registry, s.Config)

	var wg sync.WaitGroup

	GetProposalsMetrics(&wg, &sublogger, proposalsMetrics, s, s.Config, false)

	wg.Wait()
	h := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
	h.ServeHTTP(w, r)
	sublogger.Info().
		Str("method", "GET").
		Str("endpoint", "/metrics/proposals").
		Float64("request-time", time.Since(requestStart).Seconds()).
		Msg("Request processed")
}
