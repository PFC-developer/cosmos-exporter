package exporter

import (
	"context"
	"fmt"
	"math"
	"strings"

	tmservice "github.com/cosmos/cosmos-sdk/client/grpc/cmtservice"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type ServiceConfig struct {
	ConfigPath string

	Denom         string
	ListenAddress string
	NodeAddress   string
	TendermintRPC string // needed to get upgrade info
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
	PropV1     bool
	Votes      bool
}

type Service struct {
	GrpcConn *grpc.ClientConn
	//	TmRPC      *tmrpc.HTTP
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
	serviceClient := tmservice.NewServiceClient(s.GrpcConn)
	response, err := serviceClient.GetNodeInfo(
		context.Background(),
		&tmservice.GetNodeInfoRequest{},
	)
	if err != nil {
		s.Log.Fatal().Err(err).Msg("Could not query Tendermint status")
	}

	s.Log.Info().Str("network", response.GetDefaultNodeInfo().Network).Msg("Got network status from Tendermint")
	config.ChainID = response.GetDefaultNodeInfo().Network
	config.ConstLabels = map[string]string{
		"chain_id": config.ChainID,
	}
}
func (s *Service) Connect(config *ServiceConfig) error {
	var err error
	/*
		s.TmRPC, err = tmrpc.New(config.TendermintRPC, "/websocket")
		if err != nil {
			//	log.Fatal().Err(err).Msg("Could not create Tendermint client")
			return err
		}
	*/
	s.GrpcConn, err = grpc.Dial(
		config.NodeAddress,
		grpc.WithTransportCredentials(insecure.NewCredentials()))

	if err != nil {
		//log.Fatal().Err(err).Msg("Could not connect to gRPC node")
		return err
	}

	return nil
}
func (s *Service) Close() error {
	err := s.GrpcConn.Close()
	return err
}

func (s *Service) SetDenom(config *ServiceConfig) {
	// if --denom and (--denom-coefficient or --denom-exponent) are provided, use them
	// instead of fetching them via gRPC. Can be useful for networks like osmosis.
	if isUserProvidedAndHandled := s.checkAndHandleDenomInfoProvidedByUser(config); isUserProvidedAndHandled {
		return
	}

	bankClient := banktypes.NewQueryClient(s.GrpcConn)
	denoms, err := bankClient.DenomsMetadata(
		context.Background(),
		&banktypes.QueryDenomsMetadataRequest{},
	)
	if err != nil {
		s.Log.Fatal().Err(err).Msg("Error querying denom")
	}

	if len(denoms.Metadatas) == 0 {
		s.Log.Fatal().Msg("No denom infos. Try running the binary with --denom and --denom-coefficient to set them manually.")
	}

	metadata := denoms.Metadatas[0] // always using the first one
	if config.Denom == "" {         // using display currency
		config.Denom = metadata.Display
	}

	for _, unit := range metadata.DenomUnits {
		s.Log.Debug().
			Str("denom", unit.Denom).
			Uint32("exponent", unit.Exponent).
			Msg("Denom info")
		if unit.Denom == config.Denom {
			config.DenomCoefficient = math.Pow10(int(unit.Exponent))
			s.Log.Info().
				Str("denom", config.Denom).
				Float64("coefficient", config.DenomCoefficient).
				Msg("Got denom info")
			return
		}
	}

	s.Log.Fatal().Msg("Could not find the denom info")
}

func (s *Service) checkAndHandleDenomInfoProvidedByUser(config *ServiceConfig) bool {

	if config.Denom != "" {
		if config.DenomCoefficient != 1 && config.DenomExponent != 0 {
			s.Log.Fatal().Msg("denom-coefficient and denom-exponent are both provided. Must provide only one")
		}

		if config.DenomCoefficient != 1 {
			s.Log.Info().
				Str("denom", config.Denom).
				Float64("coefficient", config.DenomCoefficient).
				Msg("Using provided denom and coefficient.")
			return true
		}

		if config.DenomExponent != 0 {
			config.DenomCoefficient = math.Pow10(int(config.DenomExponent))
			s.Log.Info().
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
func (s *Service) GetLatestBlock() (float64, error) {
	serviceClient := tmservice.NewServiceClient(s.GrpcConn)
	response, err := serviceClient.GetLatestBlock(
		context.Background(),
		&tmservice.GetLatestBlockRequest{},
	)
	if err != nil {
		return 0, err
	}
	if response.GetSdkBlock() != nil {
		return float64(response.GetSdkBlock().Header.Height), nil
	} else {
		return float64(response.GetBlock().Header.Height), nil
	}
}

func (config *ServiceConfig) SetCommonParameters(cmd *cobra.Command) {

	cmd.PersistentFlags().StringVar(&config.ConfigPath, "config", "", "Config file path")
	cmd.PersistentFlags().StringVar(&config.Denom, "denom", "", "Cosmos coin denom")
	cmd.PersistentFlags().Float64Var(&config.DenomCoefficient, "denom-coefficient", 1, "Denom coefficient")
	cmd.PersistentFlags().Uint64Var(&config.DenomExponent, "denom-exponent", 0, "Denom exponent")
	cmd.PersistentFlags().StringVar(&config.ListenAddress, "listen-address", ":9300", "The address this exporter would listen on")
	cmd.PersistentFlags().StringVar(&config.NodeAddress, "node", "localhost:9090", "GRPC node address")
	cmd.PersistentFlags().StringVar(&config.LogLevel, "log-level", "info", "Logging level")
	cmd.PersistentFlags().Uint64Var(&config.Limit, "limit", 1000, "Pagination limit for gRPC requests")
	cmd.PersistentFlags().StringVar(&config.TendermintRPC, "tendermint-rpc", "http://localhost:26657", "Tendermint RPC address")
	cmd.PersistentFlags().BoolVar(&config.JSONOutput, "json", false, "Output logs as JSON")

	// some networks, like Iris, have the different prefixes for address, validator and consensus node
	cmd.PersistentFlags().StringVar(&config.Prefix, "bech-prefix", "persistence", "Bech32 global prefix")
	cmd.PersistentFlags().StringVar(&config.AccountPrefix, "bech-account-prefix", "", "Bech32 account prefix")
	cmd.PersistentFlags().StringVar(&config.AccountPubkeyPrefix, "bech-account-pubkey-prefix", "", "Bech32 pubkey account prefix")
	cmd.PersistentFlags().StringVar(&config.ValidatorPrefix, "bech-validator-prefix", "", "Bech32 validator prefix")
	cmd.PersistentFlags().StringVar(&config.ValidatorPubkeyPrefix, "bech-validator-pubkey-prefix", "", "Bech32 pubkey validator prefix")
	cmd.PersistentFlags().StringVar(&config.ConsensusNodePrefix, "bech-consensus-node-prefix", "", "Bech32 consensus node prefix")
	cmd.PersistentFlags().StringVar(&config.ConsensusNodePubkeyPrefix, "bech-consensus-node-pubkey-prefix", "", "Bech32 pubkey consensus node prefix")
	cmd.PersistentFlags().BoolVar(&config.SingleReq, "single", false, "serve info in a single call to /metrics")

	cmd.PersistentFlags().BoolVar(&config.Upgrades, "upgrades", false, "serve upgrade info in the single call to /metrics")
	cmd.PersistentFlags().BoolVar(&config.Proposals, "proposals", false, "serve active proposal info in the single call to /metrics")
	cmd.PersistentFlags().BoolVar(&config.Params, "params", false, "serve chain params info in the single call to /metrics")
	cmd.PersistentFlags().BoolVar(&config.TokenPrice, "price", true, "fetch token price")
	cmd.PersistentFlags().StringSliceVar(&config.Wallets, "wallets", nil, "serve info about passed wallets")
	cmd.PersistentFlags().StringSliceVar(&config.Validators, "validators", nil, "serve info about passed validators")
	cmd.PersistentFlags().BoolVar(&config.PropV1, "propv1", false, "use PropV1 instead of PropV1Beta calls")
	cmd.PersistentFlags().BoolVar(&config.Votes, "votes", false, "get validator votes on active proposals")
}
func (config *ServiceConfig) LogConfig(event *zerolog.Event) *zerolog.Event {
	return event.
		Str("--bech-account-prefix", config.AccountPrefix).
		Str("--bech-account-pubkey-prefix", config.AccountPubkeyPrefix).
		Str("--bech-validator-prefix", config.ValidatorPrefix).
		Str("--bech-validator-pubkey-prefix", config.ValidatorPubkeyPrefix).
		Str("--bech-consensus-node-prefix", config.ConsensusNodePrefix).
		Str("--bech-consensus-node-pubkey-prefix", config.ConsensusNodePubkeyPrefix).
		Str("--denom", config.Denom).
		Str("--denom-cofficient", fmt.Sprintf("%f", config.DenomCoefficient)).
		Str("--denom-exponent", fmt.Sprintf("%d", config.DenomExponent)).
		Str("--listen-address", config.ListenAddress).
		Str("--node", config.NodeAddress).
		Str("--log-level", config.LogLevel).
		Bool("--single", config.SingleReq).
		Str("--tendermint-rpc", config.TendermintRPC).
		Str("--wallets", strings.Join(config.Wallets[:], ",")).
		Str("--validators", strings.Join(config.Validators[:], ",")).
		Bool("--proposals", config.Proposals).
		Bool("--params", config.Params).
		Bool("--upgrades", config.Upgrades).
		Bool("--price", config.TokenPrice).
		Bool("--propv1", config.PropV1).
		Bool("--votes", config.Votes)
}

func (config *ServiceConfig) SetBechPrefixes(cmd *cobra.Command) {
	if flag, err := cmd.Flags().GetString("bech-account-prefix"); flag != "" && err == nil {
		config.AccountPrefix = flag
	} else {
		config.AccountPrefix = config.Prefix
	}

	if flag, err := cmd.Flags().GetString("bech-account-pubkey-prefix"); flag != "" && err == nil {
		config.AccountPubkeyPrefix = flag
	} else {
		config.AccountPubkeyPrefix = config.Prefix + "pub"
	}

	if flag, err := cmd.Flags().GetString("bech-validator-prefix"); flag != "" && err == nil {
		config.ValidatorPrefix = flag
	} else {
		config.ValidatorPrefix = config.Prefix + "valoper"
	}

	if flag, err := cmd.Flags().GetString("bech-validator-pubkey-prefix"); flag != "" && err == nil {
		config.ValidatorPubkeyPrefix = flag
	} else {
		config.ValidatorPubkeyPrefix = config.Prefix + "valoperpub"
	}

	if flag, err := cmd.Flags().GetString("bech-consensus-node-prefix"); flag != "" && err == nil {
		config.ConsensusNodePrefix = flag
	} else {
		config.ConsensusNodePrefix = config.Prefix + "valcons"
	}

	if flag, err := cmd.Flags().GetString("bech-consensus-node-pubkey-prefix"); flag != "" && err == nil {
		config.ConsensusNodePubkeyPrefix = flag
	} else {
		config.ConsensusNodePubkeyPrefix = config.Prefix + "valconspub"
	}
}
