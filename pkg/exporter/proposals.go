package exporter

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/types/query"
	"github.com/rs/zerolog"
	"net/http"
	"sync"
	"time"

	govtypeV1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types/v1beta1"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type ProposalsMetrics struct {
	proposalsGauge *prometheus.GaugeVec
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
				sublogger.Error().Err(err).Msg("Could not get proposals")
				return
			}

			sublogger.Debug().
				Float64("request-time", time.Since(queryStart).Seconds()).
				Msg("Finished querying proposals")
			var proposals = proposalsResponse.Proposals

			sublogger.Debug().
				Int("proposalsLength", len(proposals)).
				Msg("Proposals info")

			//cdcRegistry := codectypes.NewInterfaceRegistry()
			//cdc := codec.NewProtoCodec(cdcRegistry)
			for _, proposal := range proposals {
				var title string = ""
				if len(proposal.Metadata) > 0 {
					var metadata proposalMeta

					err := json.Unmarshal([]byte(proposal.Metadata), &metadata)
					if err != nil {
						sublogger.Error().
							Str("proposal_id", fmt.Sprint(proposal.Id)).
							Err(err).
							Msg("Could not parse proposal metadata field")
					} else {
						title = metadata.Title
					}
					sublogger.Info().
						Str("proposal_id", fmt.Sprint(proposal.Id)).
						Str("metadata", metadata.Title).
						Msg("metadata")

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
				sublogger.Error().Err(err).Msg("Could not get proposals")
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
