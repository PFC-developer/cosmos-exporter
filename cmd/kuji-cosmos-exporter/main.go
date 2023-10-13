package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	tmrpc "github.com/tendermint/tendermint/rpc/client/http"
	"google.golang.org/grpc"
	"main/pkg/exporter"
)

var config exporter.ServiceConfig
var log = zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout}).With().Timestamp().Logger()

var rootCmd = &cobra.Command{
	Use:  "cosmos-exporter",
	Long: "Scrape the data about the validators set, specific validators or wallets in the Cosmos network.",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if config.ConfigPath == "" {
			setBechPrefixes(cmd)
			return nil
		}

		viper.SetConfigFile(config.ConfigPath)
		if err := viper.ReadInConfig(); err != nil {
			if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
				log.Info().Err(err).Msg("Error reading config file")
				return err
			}
		}

		// Credits to https://carolynvanslyck.com/blog/2020/08/sting-of-the-viper/
		cmd.Flags().VisitAll(func(f *pflag.Flag) {
			if !f.Changed && viper.IsSet(f.Name) {
				val := viper.Get(f.Name)
				if err := cmd.Flags().Set(f.Name, fmt.Sprintf("%v", val)); err != nil {
					log.Fatal().Err(err).Msg("Could not set flag")
				}
			}
		})

		setBechPrefixes(cmd)

		return nil
	},
	Run: Execute,
}

func setBechPrefixes(cmd *cobra.Command) {
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

func Execute(_ *cobra.Command, _ []string) {
	logLevel, err := zerolog.ParseLevel(config.LogLevel)
	if err != nil {
		log.Fatal().Err(err).Msg("Could not parse log level")
	}

	if config.JSONOutput {
		log = zerolog.New(os.Stdout).With().Timestamp().Logger()
	}

	zerolog.SetGlobalLevel(logLevel)

	log.Info().
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
		Str("--single", fmt.Sprintf("%t", config.SingleReq)).
		Str("--wallets", strings.Join(config.Wallets[:], ",")).
		Str("--validators", strings.Join(config.Validators[:], ",")).
		Str("--oracle", fmt.Sprintf("%t", config.Oracle)).
		Str("--proposals", fmt.Sprintf("%t", config.Proposals)).
		Str("--params", fmt.Sprintf("%t", config.Params)).
		Str("--upgrades", fmt.Sprintf("%t", config.Upgrades)).
		Str("--price", fmt.Sprintf("%t", config.TokenPrice)).
		Msg("Started with following parameters")

	sdkconfig := sdk.GetConfig()
	sdkconfig.SetBech32PrefixForAccount(config.AccountPrefix, config.AccountPubkeyPrefix)
	sdkconfig.SetBech32PrefixForValidator(config.ValidatorPrefix, config.ValidatorPubkeyPrefix)
	sdkconfig.SetBech32PrefixForConsensusNode(config.ConsensusNodePrefix, config.ConsensusNodePubkeyPrefix)
	sdkconfig.Seal()

	s := &exporter.Service{}

	// Setup gRPC connection
	s.GrpcConn, err = grpc.Dial(
		config.NodeAddress,
		grpc.WithInsecure(),
	)
	if err != nil {
		log.Fatal().Err(err).Msg("Could not connect to gRPC node")
	}
	defer func(grpcConn *grpc.ClientConn) {
		err := grpcConn.Close()
		if err != nil {
			log.Fatal().Err(err).Msg("Could not close gRPC client")
		}
	}(s.GrpcConn)

	// Setup Tendermint RPC connection
	s.TmRPC, err = tmrpc.New(config.TendermintRPC, "/websocket")
	if err != nil {
		log.Fatal().Err(err).Msg("Could not create Tendermint client")
	}
	s.SetChainID(&config)
	s.SetDenom(&config)
	/*
		eventCollector, err := NewEventCollector(TendermintRPC, log, BankTransferThreshold)
		if err != nil {
			panic(err)
		}
		eventCollector.Start(cmd.Context())
	*/
	s.Params = config.Params
	s.Wallets = config.Wallets
	s.Validators = config.Validators
	s.Proposals = config.Proposals
	s.Oracle = config.Oracle
	s.Params = config.Params
	s.Upgrades = config.Upgrades
	s.Config = &config

	if config.SingleReq {
		log.Info().Msg("Starting Single Mode")
		http.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) { KujiSingleHandler(w, r, s) })
	}
	http.HandleFunc("/metrics/wallet", s.WalletHandler)
	http.HandleFunc("/metrics/validator", s.ValidatorHandler)
	http.HandleFunc("/metrics/validators", s.ValidatorsHandler)
	http.HandleFunc("/metrics/params", s.ParamsHandler)
	http.HandleFunc("/metrics/general", s.GeneralHandler)

	http.HandleFunc("/metrics/delegator", s.DelegatorHandler)
	http.HandleFunc("/metrics/proposals", s.ProposalsHandler)
	http.HandleFunc("/metrics/upgrade", s.UpgradeHandler)
	if config.Prefix == "kujira" {
		http.HandleFunc("/metrics/kujira", func(w http.ResponseWriter, r *http.Request) { KujiraMetricHandler(w, r, s) })
	}
	/*
		if Prefix == "sei" {
			http.HandleFunc("/metrics/sei", func(w http.ResponseWriter, r *http.Request) {
				OracleMetricHandler(w, r, s.grpcConn)
			})
		}

	*/
	/*
		http.HandleFunc("/metrics/event", func(w http.ResponseWriter, r *http.Request) {
			eventCollector.StreamHandler(w, r)
		})
	*/
	log.Info().Str("address", config.ListenAddress).Msg("Listening")
	err = http.ListenAndServe(config.ListenAddress, nil)
	if err != nil {
		log.Fatal().Err(err).Msg("Could not start application")
	}
}

func main() {
	rootCmd.PersistentFlags().StringVar(&config.ConfigPath, "config", "", "Config file path")
	rootCmd.PersistentFlags().StringVar(&config.Denom, "denom", "", "Cosmos coin denom")
	rootCmd.PersistentFlags().Float64Var(&config.DenomCoefficient, "denom-coefficient", 1, "Denom coefficient")
	rootCmd.PersistentFlags().Uint64Var(&config.DenomExponent, "denom-exponent", 0, "Denom exponent")
	rootCmd.PersistentFlags().StringVar(&config.ListenAddress, "listen-address", ":9300", "The address this exporter would listen on")
	rootCmd.PersistentFlags().StringVar(&config.NodeAddress, "node", "localhost:9090", "RPC node address")
	rootCmd.PersistentFlags().StringVar(&config.LogLevel, "log-level", "info", "Logging level")
	rootCmd.PersistentFlags().Uint64Var(&config.Limit, "limit", 1000, "Pagination limit for gRPC requests")
	rootCmd.PersistentFlags().StringVar(&config.TendermintRPC, "tendermint-rpc", "http://localhost:26657", "Tendermint RPC address")
	rootCmd.PersistentFlags().BoolVar(&config.JSONOutput, "json", false, "Output logs as JSON")

	// some networks, like Iris, have the different prefixes for address, validator and consensus node
	rootCmd.PersistentFlags().StringVar(&config.Prefix, "bech-prefix", "persistence", "Bech32 global prefix")
	rootCmd.PersistentFlags().StringVar(&config.AccountPrefix, "bech-account-prefix", "", "Bech32 account prefix")
	rootCmd.PersistentFlags().StringVar(&config.AccountPubkeyPrefix, "bech-account-pubkey-prefix", "", "Bech32 pubkey account prefix")
	rootCmd.PersistentFlags().StringVar(&config.ValidatorPrefix, "bech-validator-prefix", "", "Bech32 validator prefix")
	rootCmd.PersistentFlags().StringVar(&config.ValidatorPubkeyPrefix, "bech-validator-pubkey-prefix", "", "Bech32 pubkey validator prefix")
	rootCmd.PersistentFlags().StringVar(&config.ConsensusNodePrefix, "bech-consensus-node-prefix", "", "Bech32 consensus node prefix")
	rootCmd.PersistentFlags().StringVar(&config.ConsensusNodePubkeyPrefix, "bech-consensus-node-pubkey-prefix", "", "Bech32 pubkey consensus node prefix")
	rootCmd.PersistentFlags().BoolVar(&config.SingleReq, "single", false, "serve info in a single call to /metrics")
	rootCmd.PersistentFlags().BoolVar(&config.Oracle, "oracle", false, "serve oracle info in the single call to /metrics")
	rootCmd.PersistentFlags().BoolVar(&config.Upgrades, "upgrades", false, "serve upgrade info in the single call to /metrics")
	rootCmd.PersistentFlags().BoolVar(&config.Proposals, "proposals", false, "serve active proposal info in the single call to /metrics")
	rootCmd.PersistentFlags().BoolVar(&config.Params, "params", false, "serve chain params info in the single call to /metrics")
	rootCmd.PersistentFlags().BoolVar(&config.TokenPrice, "price", true, "fetch token price")
	rootCmd.PersistentFlags().StringSliceVar(&config.Wallets, "wallets", nil, "serve info about passed wallets")
	rootCmd.PersistentFlags().StringSliceVar(&config.Validators, "validators", nil, "serve info about passed validators")

	rootCmd.PersistentFlags().Float64Var(&config.BankTransferThreshold, "bank-transfer-threshold", 1e13, "The threshold for which to track bank transfers")

	if err := rootCmd.Execute(); err != nil {
		log.Fatal().Err(err).Msg("Could not start application")
	}
}
