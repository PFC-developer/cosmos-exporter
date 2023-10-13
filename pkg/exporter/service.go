package exporter

import (
	"context"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	tmrpc "github.com/tendermint/tendermint/rpc/client/http"
	"google.golang.org/grpc"
	"math"
)

type ServiceConfig struct {
	ConfigPath string

	Denom         string
	ListenAddress string
	NodeAddress   string
	TendermintRPC string
	LogLevel      string
	JSONOutput    bool
	Limit         uint64

	Prefix                    string
	AccountPrefix             string
	AccountPubkeyPrefix       string
	ValidatorPrefix           string
	ValidatorPubkeyPrefix     string
	ConsensusNodePrefix       string
	ConsensusNodePubkeyPrefix string

	BankTransferThreshold float64

	ChainID          string
	ConstLabels      map[string]string
	DenomCoefficient float64
	DenomExponent    uint64

	// SingleReq bundle up multiple requests into a single /metrics
	SingleReq  bool
	Wallets    []string
	Validators []string
	Oracle     bool
	Upgrades   bool
	Proposals  bool
	Params     bool
	TokenPrice bool
}

type Service struct {
	GrpcConn   *grpc.ClientConn
	TmRPC      *tmrpc.HTTP
	Wallets    []string
	Validators []string
	Oracle     bool
	Upgrades   bool
	Proposals  bool
	Params     bool
	Config     *ServiceConfig
	Log        zerolog.Logger
}

func (s *Service) SetChainID(config *ServiceConfig) {
	status, err := s.TmRPC.Status(context.Background())
	if err != nil {
		log.Fatal().Err(err).Msg("Could not query Tendermint status")
	}

	log.Info().Str("network", status.NodeInfo.Network).Msg("Got network status from Tendermint")
	config.ChainID = status.NodeInfo.Network
	config.ConstLabels = map[string]string{
		"chain_id": config.ChainID,
	}
}

func (s *Service) SetDenom(config *ServiceConfig) {
	// if --denom and (--denom-coefficient or --denom-exponent) are provided, use them
	// instead of fetching them via gRPC. Can be useful for networks like osmosis.
	if isUserProvidedAndHandled := checkAndHandleDenomInfoProvidedByUser(config); isUserProvidedAndHandled {
		return
	}

	bankClient := banktypes.NewQueryClient(s.GrpcConn)
	denoms, err := bankClient.DenomsMetadata(
		context.Background(),
		&banktypes.QueryDenomsMetadataRequest{},
	)
	if err != nil {
		log.Fatal().Err(err).Msg("Error querying denom")
	}

	if len(denoms.Metadatas) == 0 {
		log.Fatal().Msg("No denom infos. Try running the binary with --denom and --denom-coefficient to set them manually.")
	}

	metadata := denoms.Metadatas[0] // always using the first one
	if config.Denom == "" {         // using display currency
		config.Denom = metadata.Display
	}

	for _, unit := range metadata.DenomUnits {
		log.Debug().
			Str("denom", unit.Denom).
			Uint32("exponent", unit.Exponent).
			Msg("Denom info")
		if unit.Denom == config.Denom {
			config.DenomCoefficient = math.Pow10(int(unit.Exponent))
			log.Info().
				Str("denom", config.Denom).
				Float64("coefficient", config.DenomCoefficient).
				Msg("Got denom info")
			return
		}
	}

	log.Fatal().Msg("Could not find the denom info")
}

func checkAndHandleDenomInfoProvidedByUser(config *ServiceConfig) bool {

	if config.Denom != "" {
		if config.DenomCoefficient != 1 && config.DenomExponent != 0 {
			log.Fatal().Msg("denom-coefficient and denom-exponent are both provided. Must provide only one")
		}

		if config.DenomCoefficient != 1 {
			log.Info().
				Str("denom", config.Denom).
				Float64("coefficient", config.DenomCoefficient).
				Msg("Using provided denom and coefficient.")
			return true
		}

		if config.DenomExponent != 0 {
			config.DenomCoefficient = math.Pow10(int(config.DenomExponent))
			log.Info().
				Str("denom", config.Denom).
				Uint64("exponent", config.DenomExponent).
				Float64("calculated coefficient", config.DenomCoefficient).
				Msg("Using provided denom and denom exponent and calculating coefficient.")
			return true
		}

		return false
	}

	return false

}
